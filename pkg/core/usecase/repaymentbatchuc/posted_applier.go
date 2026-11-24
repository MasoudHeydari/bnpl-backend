package repaymentbatchuc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/collectionpayment"
	extcollection "github.com/parstis/bnpl-backend/pkg/core/external/collectionpayment"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// DefaultPostedApplier is the BNPL-side projection of an authoritative FC
// posting. It never creates/updates an FC journal or contacts a remote system.
// The caller supplies one transaction that also advances the batch to posted.
//
// Settlement evidence is written before installment MarkPaid so the existing
// payment-snapshot triggers accept batch settlement without synthetic
// PaymentAttempts. Legacy single-installment PaymentAttempt evidence remains
// valid for the installment-repayment path.
type DefaultPostedApplier struct {
	customer     repo.CustomerProfile
	contracts    repo.Contract
	installments repo.Installment
	charges      repo.InstallmentCharge
	accounts     repo.CreditAccount
	attempts     repo.PaymentAttempt
	batches      repo.RepaymentBatch
	ids          identifier.Generator
	clock        clock.Clock
}

var _ PostedApplier = (*DefaultPostedApplier)(nil)

// NewDefaultPostedApplier constructs the local FC posting applier.
func NewDefaultPostedApplier(
	customer repo.CustomerProfile,
	contracts repo.Contract,
	installments repo.Installment,
	charges repo.InstallmentCharge,
	accounts repo.CreditAccount,
	attempts repo.PaymentAttempt,
	batches repo.RepaymentBatch,
	ids identifier.Generator,
	clk clock.Clock,
) (*DefaultPostedApplier, error) {
	if customer == nil || contracts == nil || installments == nil || charges == nil ||
		accounts == nil || attempts == nil || batches == nil || ids == nil || clk == nil {
		return nil, fmt.Errorf("repaymentbatchuc.NewDefaultPostedApplier: missing dependency")
	}

	return &DefaultPostedApplier{
		customer: customer, contracts: contracts, installments: installments,
		charges: charges, accounts: accounts, attempts: attempts,
		batches: batches, ids: ids, clock: clk,
	}, nil
}

// ApplyPosted records immutable settlement evidence then settles
// installments, charges, exposure, and eligible contracts in one TX.
func (a *DefaultPostedApplier) ApplyPosted(
	ctx context.Context, tx repo.Tx, plan collectionpayment.Plan, result extcollection.Result,
) error {
	const op = "repaymentbatchuc.DefaultPostedApplier.ApplyPosted"
	if err := extcollection.VerifyPostedAllocations(plan, result); err != nil {
		return err
	}

	if result.PostedAt == nil || result.PostedAt.IsZero() {
		return cerr.Conflict(fmt.Errorf("%s: missing FC posting timestamp", op))
	}

	// Lock customer first, then all contracts, installments, charges and credit
	// accounts in deterministic order. This serializes settlement with other
	// customer-scoped payment and origination workflows.
	customerQ := a.customer.Tx(tx)
	profile, err := customerQ.LockByID(ctx, plan.CustomerID)
	if err != nil {
		return err
	}

	if profile.ExternalCustomerID != plan.ExternalCustomerID {
		return cerr.Conflict(fmt.Errorf("%s: FC obligor does not match customer", op))
	}

	cq := a.contracts.Tx(tx)
	iq := a.installments.Tx(tx)
	chq := a.charges.Tx(tx)
	aq := a.accounts.Tx(tx)
	pq := a.attempts.Tx(tx)
	batchesQ := a.batches.Tx(tx)

	contractIDs := make([]model.ContractID, 0)
	seenContracts := make(map[model.ContractID]struct{})
	accountIDs := make([]model.CreditAccountID, 0)
	seenAccounts := make(map[model.CreditAccountID]struct{})
	installmentIDs := make([]model.InstallmentID, 0, len(plan.Items))
	for _, item := range plan.Items {
		if _, ok := seenContracts[item.ContractID]; !ok {
			seenContracts[item.ContractID] = struct{}{}
			contractIDs = append(contractIDs, item.ContractID)
		}

		if _, ok := seenAccounts[item.CreditAccountID]; !ok {
			seenAccounts[item.CreditAccountID] = struct{}{}
			accountIDs = append(accountIDs, item.CreditAccountID)
		}

		installmentIDs = append(installmentIDs, item.InstallmentID)
	}

	sort.Slice(contractIDs, func(i, j int) bool { return contractIDs[i].String() < contractIDs[j].String() })
	sort.Slice(installmentIDs, func(i, j int) bool { return installmentIDs[i].String() < installmentIDs[j].String() })
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i].String() < accountIDs[j].String() })

	lockedContracts := make(map[model.ContractID]model.Contract, len(contractIDs))
	for _, id := range contractIDs {
		c, err := cq.LockByID(ctx, id)
		if err != nil {
			return err
		}

		if c.Status != model.ActiveContractStatus || c.CustomerID != plan.CustomerID ||
			c.Currency != plan.Currency {
			return cerr.Conflict(fmt.Errorf("%s: contract is not an active obligation", op))
		}

		lockedContracts[id] = c
	}

	lockedInstallments := make(map[model.InstallmentID]model.Installment, len(installmentIDs))
	for _, id := range installmentIDs {
		inst, err := iq.LockByID(ctx, id)
		if err != nil {
			return err
		}

		// Idempotent replay: an already-paid installment from a prior successful
		// application of this same FC posting must not fail the whole batch.
		if inst.Status == model.PaidInstallmentStatus {
			lockedInstallments[id] = inst
			continue
		}

		if !inst.Status.IsActive() || inst.CustomerID != plan.CustomerID ||
			inst.Currency != plan.Currency {
			return cerr.Conflict(fmt.Errorf("%s: installment is not payable", op))
		}

		lockedInstallments[id] = inst
	}

	lockedCharges, err := chq.LockByInstallmentIDs(ctx, installmentIDs)
	if err != nil {
		return err
	}

	chargesByInstallment := make(map[model.InstallmentID][]model.InstallmentCharge, len(installmentIDs))
	for _, charge := range lockedCharges {
		chargesByInstallment[charge.InstallmentID] = append(chargesByInstallment[charge.InstallmentID], charge)
	}

	lockedAccounts := make(map[model.CreditAccountID]model.CreditAccount, len(accountIDs))
	for _, id := range accountIDs {
		account, err := aq.LockByID(ctx, id)
		if err != nil {
			return err
		}

		if !model.CreditBelongsToCustomer(account, profile, plan.Currency) {
			return cerr.Conflict(fmt.Errorf("%s: credit account does not belong to customer", op))
		}

		lockedAccounts[id] = account
	}

	// posted_at is an accounting timestamp, not a synthetic provider_paid_at.
	// It is used only as local application time when FC does not supply a
	// verified provider payment timestamp.
	effectivePaidAt := result.PostedAt.UTC()
	if result.ProviderPaidAt != nil && !result.ProviderPaidAt.IsZero() {
		effectivePaidAt = result.ProviderPaidAt.UTC()
	}

	now := a.clock.Now().UTC()
	sort.Slice(plan.Items, func(i, j int) bool {
		return plan.Items[i].InstallmentID.String() < plan.Items[j].InstallmentID.String()
	})

	allocations := make([]repo.RepaymentBatchSettlementAllocation, 0, len(plan.Items))
	payableByInstallment := make(map[model.InstallmentID]uint64, len(plan.Items))
	for _, item := range plan.Items {
		contract := lockedContracts[item.ContractID]
		installment := lockedInstallments[item.InstallmentID]
		account := lockedAccounts[item.CreditAccountID]
		if installment.Status == model.PaidInstallmentStatus {
			continue
		}

		if installment.ContractID == nil || *installment.ContractID != contract.ID ||
			contract.CreditAccountID != account.ID || account.CreditKind != item.CreditKind {
			return cerr.Conflict(fmt.Errorf("%s: installment/contract/product relationship changed", op))
		}

		charges := chargesByInstallment[item.InstallmentID]
		currentPayable, ok, err := model.CustomerPayableAmount(installment, charges)
		if err != nil {
			return err
		}

		var snapshotted uint64
		var postingRefs []string
		for _, component := range item.Components {
			if snapshotted > ^uint64(0)-component.NetMinor {
				return cerr.Conflict(fmt.Errorf("%s: item amount overflow", op))
			}

			snapshotted += component.NetMinor
			for _, allocation := range component.Allocations {
				for _, posted := range result.PostedAllocations {
					if posted.AllocationID == allocation.ID {
						ref := strings.TrimSpace(posted.PostingReference)
						if ref != "" {
							postingRefs = append(postingRefs, ref)
						}
					}
				}
			}
		}

		if !ok || snapshotted != currentPayable {
			return cerr.Conflict(fmt.Errorf("%s: collection snapshot differs from current payable obligation", op))
		}

		if contract.RemainingPrincipalAmountMinor < installment.PrincipalAmountMinor ||
			account.ExposureAmountMinor < installment.PrincipalAmountMinor {
			return cerr.Conflict(fmt.Errorf("%s: principal/exposure inconsistent with FC posting", op))
		}

		payableByInstallment[item.InstallmentID] = snapshotted
		postingReference := strings.Join(postingRefs, "|")
		if postingReference == "" {
			// Deterministic fallback still binds evidence to this FC payment and
			// installment so uniqueness and audit remain enforceable.
			postingReference = fmt.Sprintf("%s:%s", result.CollectionPaymentID, item.InstallmentID.String())
		}

		allocID, idErr := uuid.Parse(a.ids.Generate())
		if idErr != nil {
			return cerr.InternalError(fmt.Errorf("%s: generate settlement allocation id: %w", op, idErr))
		}

		allocations = append(allocations, repo.RepaymentBatchSettlementAllocation{
			ID:               allocID,
			InstallmentID:    item.InstallmentID,
			AmountMinor:      snapshotted,
			Currency:         plan.Currency,
			PostingReference: postingReference,
		})
	}

	if len(allocations) > 0 {
		settlementID, idErr := uuid.Parse(a.ids.Generate())
		if idErr != nil {
			return cerr.InternalError(fmt.Errorf("%s: generate settlement id: %w", op, idErr))
		}

		if err := batchesQ.RecordSettlement(
			ctx,
			settlementID,
			plan.ID,
			result.CollectionPaymentID,
			result.SnapshotRevision,
			plan.Currency,
			plan.TotalMinor,
			*result.PostedAt,
			now,
			allocations,
		); err != nil {
			return err
		}
	}

	for _, item := range plan.Items {
		installment := lockedInstallments[item.InstallmentID]
		if installment.Status == model.PaidInstallmentStatus {
			continue
		}

		contract := lockedContracts[item.ContractID]
		account := lockedAccounts[item.CreditAccountID]
		charges := chargesByInstallment[item.InstallmentID]
		snapshotted := payableByInstallment[item.InstallmentID]

		var ids []model.InstallmentChargeID
		for _, charge := range charges {
			if model.IsCustomerPayableCharge(charge) {
				ids = append(ids, charge.ID)
			}
		}

		if len(ids) > 0 {
			affected, err := chq.MarkSnapshottedChargeIDsPaid(ctx, ids, effectivePaidAt, now)
			if err != nil {
				return err
			}

			if affected != int64(len(ids)) {
				return cerr.Conflict(fmt.Errorf("%s: charge settlement changed concurrently", op))
			}
		}

		if _, err := iq.MarkPaid(ctx, installment.ID, contract.ID, snapshotted, effectivePaidAt, now); err != nil {
			return err
		}

		if installment.PrincipalAmountMinor > 0 {
			updated, err := aq.ReduceExposureIfSufficient(ctx, account.ID, installment.PrincipalAmountMinor, now)
			if err != nil {
				return err
			}

			lockedAccounts[account.ID] = updated
			if _, err := aq.AppendTransaction(ctx, account.ID,
				model.ExposureDecreasedCreditTransactionType, installment.PrincipalAmountMinor,
				contract.Currency, model.InstallmentPaidCreditTransactionReason,
				model.InstallmentCreditTransactionReferenceType, installment.ID.UUID(),
				effectivePaidAt, now); err != nil {
				return err
			}

			updatedContract, err := cq.ReduceRemainingPrincipal(ctx, contract.ID, installment.PrincipalAmountMinor, now)
			if err != nil {
				return err
			}

			lockedContracts[contract.ID] = updatedContract
		}
	}

	// Complete contracts only after every installment in the batch was marked
	// paid. Multiple installments of one contract are settled atomically.
	for _, id := range contractIDs {
		contract, err := cq.GetByID(ctx, id)
		if err != nil {
			return err
		}

		if contract.RemainingPrincipalAmountMinor != 0 {
			continue
		}

		all, err := iq.ListByContractID(ctx, id)
		if err != nil {
			return err
		}

		if len(all) != contract.InstallmentCount {
			return cerr.Conflict(fmt.Errorf("%s: contract installment count mismatch", op))
		}

		allPaid := true
		for _, installment := range all {
			if installment.Status != model.PaidInstallmentStatus {
				allPaid = false
				break
			}
		}

		if !allPaid {
			continue
		}

		pendingCharges, err := chq.CountUnpaidCustomerDebitChargesByContractID(ctx, id)
		if err != nil {
			return err
		}

		if pendingCharges > 0 {
			continue
		}

		blocking, err := pq.HasBlockingAttemptForContract(ctx, id)
		if err != nil {
			return err
		}

		if blocking {
			continue
		}

		if _, err := cq.MarkCompleted(ctx, id, now, now); err != nil {
			return err
		}
	}

	return nil
}
