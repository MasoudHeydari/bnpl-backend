package repaymentbatchuc

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/collectionpayment"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Obligation is the BNPL-owned contractual snapshot used for building a
// collection. No Financial Core claim/recognition reference is inferred from
// MerchantEntitlementID, and no customer request contains allocation rules.
type Obligation struct {
	Installment model.Installment
	Contract model.Contract
	CreditAccount model.CreditAccount
	Merchant model.MerchantProfile
	Charges []model.InstallmentCharge
	PayableMinor uint64
}

// FinancialInstructions contain FC-authorized economic references for an
// existing BNPL obligation. The resolver must obtain the versioned relationship,
// funding shares, recognition reference and distribution rules from a trusted
// Financial provisioning integration, NEVER from the customer's HTTP payload.
type FinancialInstructions struct {
	FundingShares []collectionpayment.FundingShare
	Components []collectionpayment.Component
}

// FinancialInstructionsResolver supplies only financial terms; BNPL owns
// customer, contract, installment, merchant and product identity. The FC v2
// handoff does not yet publish a concrete RPC for this provisioning step.
type FinancialInstructionsResolver interface {
	Resolve(ctx context.Context, obligations []Obligation) ([]FinancialInstructions, error)
}

// WithPlanCreation configures the BNPL-owned source repositories and UUID port.
// This intentionally does not enable plan creation unless a trusted Financial
// resolver has also been supplied via WithFinancialInstructionsResolver.
func (uc *UseCase) WithPlanCreation(
	installments repo.Installment,
	contracts repo.Contract,
	accounts repo.CreditAccount,
	merchants repo.MerchantProfile,
	charges repo.InstallmentCharge,
	generator identifier.Generator,
) *UseCase {
	uc.installments = installments
	uc.contracts = contracts
	uc.accounts = accounts
	uc.merchants = merchants
	uc.charges = charges
	uc.idGenerator = generator
	return uc
}

// WithFinancialInstructionsResolver must be called only with a trusted adapter
// whose data originates from authorized Financial provisioning. A missing
// resolver always fails closed and does not reserve customer installments.
func (uc *UseCase) WithFinancialInstructionsResolver(resolver FinancialInstructionsResolver) *UseCase {
	uc.instructions = resolver
	return uc
}

func (uc *UseCase) newID() (uuid.UUID, error) {
	if uc.idGenerator == nil {
		return uuid.Nil, cerr.ServiceUnavailable(fmt.Errorf("repayment ID generator unavailable"))
	}
	id, err := uuid.Parse(uc.idGenerator.Generate())
	if err != nil || id == uuid.Nil {
		return uuid.Nil, cerr.InternalError(fmt.Errorf("repayment ID generator returned invalid UUID"))
	}
	return id, nil
}

func sameInstallmentSelection(plan collectionpayment.Plan, requested []model.InstallmentID) bool {
	if len(plan.Items) != len(requested) {
		return false
	}
	a := make([]string, 0, len(plan.Items))
	b := make([]string, 0, len(requested))
	for _, item := range plan.Items {
		a = append(a, item.InstallmentID.String())
	}
	for _, id := range requested {
		b = append(b, id.String())
	}
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// CreateForCustomer prepares exactly the customer's selected installments as
// ONE immutable Financial batch. The HTTP boundary supplies only IDs and an
// idempotency key; all amounts and entitlement references come from BNPL/FC.
//
// Every DB read finishes before the Financial resolver or Prepare RPC is called.
// The repository revalidates selected installments under SQL row locks when
// writing the immutable snapshot and claims. A concurrent legacy payment cannot
// bypass those claims.
func (uc *UseCase) CreateForCustomer(
	ctx context.Context,
	subject model.ExternalSubjectID,
	selected []model.InstallmentID,
	key string,
) (repo.RepaymentBatchRow, error) {
	const op = "repaymentbatchuc.UseCase.CreateForCustomer"
	if subject.IsZero() || key == "" || len(key) > 256 ||
		len(selected) == 0 || len(selected) > 50 {
		return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("%s: invalid selection or idempotency key", op))
	}
	unique := make(map[model.InstallmentID]struct{}, len(selected))
	for _, id := range selected {
		if id.IsZero() {
			return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("%s: empty installment id", op))
		}
		if _, exists := unique[id]; exists {
			return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("%s: duplicate installment id", op))
		}
		unique[id] = struct{}{}
	}
	if uc.customer == nil || uc.installments == nil || uc.contracts == nil ||
		uc.accounts == nil || uc.merchants == nil || uc.charges == nil {
		return repo.RepaymentBatchRow{}, cerr.InternalError(fmt.Errorf("%s: missing BNPL obligation repositories", op))
	}

	var (
		profile model.CustomerProfile
		existing repo.RepaymentBatchRow
		replayed bool
		obligations []Obligation
	)
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var loadErr error
		profile, loadErr = uc.customer.Conn(conn).GetByExternalSubjectID(ctx, subject)
		if loadErr != nil {
			return loadErr
		}
		if !profile.IsActive() {
			return cerr.Conflict(fmt.Errorf("%s: customer profile is not active", op))
		}
		existing, loadErr = uc.batches.Conn(conn).GetByCustomerAndIdempotencyKey(ctx, profile.ID, key)
		if loadErr == nil {
			plan, decodeErr := planFromRow(existing)
			if decodeErr != nil {
				return decodeErr
			}
			if !sameInstallmentSelection(plan, selected) {
				return cerr.Conflict(fmt.Errorf("%s: idempotency key already used for another installment selection", op))
			}
			replayed = true
			return nil
		}
		if !cerr.IsCode(loadErr, cerr.CodeResourceNotFound) {
			return loadErr
		}

		obligations = make([]Obligation, 0, len(selected))
		for _, id := range selected {
			installment, getErr := uc.installments.Conn(conn).GetByID(ctx, id)
			if getErr != nil {
				return getErr
			}
			if !installment.Status.IsActive() || installment.ContractID == nil ||
				installment.CustomerID != profile.ID {
				return cerr.Conflict(fmt.Errorf("%s: selected installment is not payable by customer", op))
			}
			contract, getErr := uc.contracts.Conn(conn).GetByID(ctx, *installment.ContractID)
			if getErr != nil {
				return getErr
			}
			if contract.CustomerID != profile.ID || contract.Status != model.ActiveContractStatus ||
				contract.Currency != installment.Currency {
				return cerr.Conflict(fmt.Errorf("%s: contract is not a payable customer obligation", op))
			}
			account, getErr := uc.accounts.Conn(conn).GetByID(ctx, contract.CreditAccountID)
			if getErr != nil {
				return getErr
			}
			if account.CustomerID != profile.ID || account.Currency != installment.Currency {
				return cerr.Conflict(fmt.Errorf("%s: credit account/contract identity mismatch", op))
			}
			merchant, getErr := uc.merchants.Conn(conn).GetByID(ctx, contract.MerchantProfileID)
			if getErr != nil {
				return getErr
			}
			chargeRows, getErr := uc.charges.Conn(conn).ListByInstallmentIDs(ctx, []model.InstallmentID{id})
			if getErr != nil {
				return getErr
			}
			payable, ok, getErr := model.CustomerPayableAmount(installment, chargeRows)
			if getErr != nil {
				return getErr
			}
			if !ok || payable == 0 {
				return cerr.Conflict(fmt.Errorf("%s: installment has no payable amount", op))
			}
			if contract.FinancialMerchantEntitlementID == nil ||
				*contract.FinancialMerchantEntitlementID == "" {
				return cerr.ServiceUnavailable(fmt.Errorf("%s: existing entitlement is not bound by FC", op))
			}
			obligations = append(obligations, Obligation{
				Installment: installment,
				Contract: contract,
				CreditAccount: account,
				Merchant: merchant,
				Charges: chargeRows,
				PayableMinor: payable,
			})
		}
		return nil
	})
	if err != nil {
		return repo.RepaymentBatchRow{}, err
	}
	if replayed {
		// A retry never produces new item IDs or a second FC source. Reconcile
		// handles a previously lost Prepare response using the persisted ID.
		return existing, nil
	}
	if uc.instructions == nil {
		return repo.RepaymentBatchRow{}, cerr.ServiceUnavailable(fmt.Errorf("%s: FC financial provisioning resolver is not configured", op))
	}

	// External authorization/resolution is deliberately outside the DB tx.
	instructions, err := uc.instructions.Resolve(ctx, obligations)
	if err != nil {
		return repo.RepaymentBatchRow{}, fmt.Errorf("%s: resolve authorized financial instructions: %w", op, err)
	}
	if len(instructions) != len(obligations) {
		return repo.RepaymentBatchRow{}, cerr.InternalError(fmt.Errorf("%s: incorrect number of FC instructions", op))
	}

	batchID, err := uc.newID()
	if err != nil {
		return repo.RepaymentBatchRow{}, err
	}
	plan := collectionpayment.Plan{
		ID: batchID, CustomerID: profile.ID,
		ExternalCustomerID: profile.ExternalCustomerID,
		Currency: obligations[0].Installment.Currency,
		Description: "BNPL installment repayment",
	}
	for i, obligation := range obligations {
		inst := obligation.Installment
		if inst.Currency != plan.Currency {
			return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("%s: mixed currencies cannot share a collection", op))
		}
		itemID, idErr := uc.newID()
		if idErr != nil {
			return repo.RepaymentBatchRow{}, idErr
		}
		resolved := instructions[i]
		item := collectionpayment.Item{
			ID: itemID, InstallmentID: inst.ID,
			ContractID: obligation.Contract.ID,
			CreditAccountID: obligation.CreditAccount.ID,
			CreditKind: model.EffectiveCreditKind(obligation.CreditAccount.CreditKind),
			MerchantID: obligation.Merchant.ExternalMerchantID,
			MerchantEntitlementID: *obligation.Contract.FinancialMerchantEntitlementID,
			FundingShares: resolved.FundingShares,
			Components: resolved.Components,
		}
		// Product rules own the payable amount. FC-provisioned instructions
		// cannot add or remove cash from the accepted BNPL installment.
		var net uint64
		var principal uint64
		for j := range item.Components {
			comp := &item.Components[j]
			if comp.Kind == collectionpayment.PrincipalComponent {
				if principal > inst.PrincipalAmountMinor ||
					comp.GrossMinor > inst.PrincipalAmountMinor-principal {
					return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("%s: FC instructions exceed recognized principal", op))
				}
				principal += comp.GrossMinor
			}
			if comp.NetMinor > ^uint64(0)-net {
				return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("%s: collection item amount overflow", op))
			}
			net += comp.NetMinor
		}
		if principal != inst.PrincipalAmountMinor || net != obligation.PayableMinor {
			return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("%s: FC instructions do not match installment payable amount", op))
		}
		if obligation.PayableMinor > ^uint64(0)-plan.TotalMinor {
			return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("%s: aggregate amount overflow", op))
		}
		plan.TotalMinor += obligation.PayableMinor
		plan.Items = append(plan.Items, item)
	}
	if err := plan.Validate(); err != nil {
		return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("%s: invalid authorized plan: %w", op, err))
	}

	// Claims are committed before the network call. If FC is unavailable
	// afterwards, return the durable batch ID for safe future reconciliation.
	_, prepareErr := uc.PrepareDurably(ctx, plan, key)
	row, loadErr := uc.load(ctx, batchID)
	if loadErr == nil {
		return row, nil
	}
	if prepareErr != nil {
		// Two concurrent requests may both miss the preflight idempotency
		// lookup. The unique (customer,idempotency_key) index selects one
		// winner. A loser resolves that persisted winner and verifies that
		// the caller selected the same installments. It must never mint an
		// alternate FC source or allocate the installment twice.
		var winner repo.RepaymentBatchRow
		recoveryErr := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			var err error
			winner, err = uc.batches.Conn(conn).GetByCustomerAndIdempotencyKey(ctx, profile.ID, key)
			return err
		})
		if recoveryErr == nil {
			winnerPlan, err := planFromRow(winner)
			if err != nil {
				return repo.RepaymentBatchRow{}, err
			}
			if !sameInstallmentSelection(winnerPlan, selected) {
				return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("%s: idempotency key reused for another installment selection", op))
			}
			return winner, nil
		}
		return repo.RepaymentBatchRow{}, prepareErr
	}
	return repo.RepaymentBatchRow{}, loadErr
}

