package contractuc

import (
	"context"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type merchantEntitlementRecord struct {
	MerchantEntitlementID string
	ReceivableID          string
	AlreadyBound          bool
}

// EnsureMerchantEntitlement records the economic recognition for an activated
// checkout in Financial Core and durably binds the returned entitlement ids.
//
// The Financial Core call is deliberately outside a database transaction. This
// method performs one synchronous external call per invocation and contains no
// retry loop. If the caller invokes the operation again, the stable contract
// source reference lets Financial Core return its idempotent result safely.
func (uc *UseCase) EnsureMerchantEntitlement(
	ctx context.Context,
	contractID model.ContractID,
) error {
	recorded, err := uc.recordMerchantEntitlement(
		ctx,
		contractID,
		model.ActiveContractStatus,
	)
	if err != nil {
		return err
	}
	if recorded.AlreadyBound {
		return nil
	}

	now := uc.clock.Now().UTC()
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			contractQ := uc.contract.Tx(tx)
			if _, bindErr := contractQ.BindMerchantEntitlementIfAbsent(
				ctx,
				contractID,
				recorded.MerchantEntitlementID,
				now,
			); bindErr != nil {
				return bindErr
			}

			_, bindErr := contractQ.BindReceivableIfAbsent(
				ctx,
				contractID,
				recorded.ReceivableID,
				now,
			)
			return bindErr
		})
	})
}

// recordMerchantEntitlement performs the single synchronous Financial Core
// recognition call used by both post-payment activation and merchant-funding
// activation. It never opens a transaction around the network call and never
// retries it.
func (uc *UseCase) recordMerchantEntitlement(
	ctx context.Context,
	contractID model.ContractID,
	allowedStatuses ...model.ContractStatus,
) (merchantEntitlementRecord, error) {
	const op = "contractuc.UseCase.recordMerchantEntitlement"

	var (
		contractRow model.Contract
		customer    model.CustomerProfile
		merchant    model.MerchantProfile
		first       model.Installment
		selected    checkout.Policy
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var err error
		contractQ := uc.contract.Conn(conn)
		contractRow, err = contractQ.GetByID(ctx, contractID)
		if err != nil {
			return err
		}

		if !contractStatusAllowed(contractRow.Status, allowedStatuses) {
			return cerr.Conflict(
				fmt.Errorf("%s: contract status does not allow merchant entitlement recording", op),
				map[string]any{"contract_id": contractID, "status": contractRow.Status},
			)
		}

		if contractHasFinancialRecognition(contractRow) {
			return nil
		}

		decisionQ := uc.decision.Conn(conn)
		decision, err := decisionQ.GetByID(ctx, contractRow.DecisionID)
		if err != nil {
			return err
		}
		selected, err = uc.checkoutPolicies.Resolve(policy.Ref{ID: decision.PolicyID, Version: decision.PolicyVersion})
		if err != nil {
			return fmt.Errorf("%s: resolve persisted checkout policy: %w", op, err)
		}

		customerProfileQ := uc.customerProfile.Conn(conn)
		customer, err = customerProfileQ.GetByID(ctx, contractRow.CustomerID)
		if err != nil {
			return err
		}

		merchantProfileQ := uc.merchantProfile.Conn(conn)
		merchant, err = merchantProfileQ.GetByID(ctx, contractRow.MerchantProfileID)
		if err != nil {
			return err
		}

		installmentQ := uc.installment.Conn(conn)
		installments, err := installmentQ.ListByContractID(ctx, contractID)
		if err != nil {
			return err
		}

		var ok bool
		first, ok = firstInstallment(installments)
		if !ok {
			return cerr.InternalError(fmt.Errorf("%s: contract first installment is missing", op))
		}

		return nil
	})
	if err != nil {
		return merchantEntitlementRecord{}, err
	}

	if contractHasFinancialRecognition(contractRow) {
		return merchantEntitlementRecord{
			MerchantEntitlementID: strings.TrimSpace(*contractRow.FinancialMerchantEntitlementID),
			ReceivableID:          strings.TrimSpace(*contractRow.FinancialReceivableID),
			AlreadyBound:          true,
		}, nil
	}

	plan, err := buildContractPlan(selected, model.CheckoutDecisionAllocation{
		RequestedAmountMinor:        contractRow.RequestedAmountMinor,
		ApprovedFinancedAmountMinor: contractRow.ApprovedFinancedAmountMinor,
		UpfrontAmountMinor:          contractRow.UpfrontAmountMinor,
	}, first)
	if err != nil {
		return merchantEntitlementRecord{}, cerr.InternalError(fmt.Errorf("%s: rebuild contract plan: %w", op, err))
	}

	collectedCash := plan.CollectedCashAmountMinor
	receivable := plan.ReceivableAmountMinor
	if receivable != contractRow.RemainingPrincipalAmountMinor {
		return merchantEntitlementRecord{}, cerr.InternalError(
			fmt.Errorf("%s: policy receivable does not match contract remaining principal", op),
		)
	}
	merchantPayable, err := model.AddAmountMinor(collectedCash, receivable)
	if err != nil {
		return merchantEntitlementRecord{}, cerr.InternalError(fmt.Errorf("%s: merchant payable: %w", op, err))
	}
	if merchantPayable != contractRow.RequestedAmountMinor {
		return merchantEntitlementRecord{}, cerr.InternalError(
			fmt.Errorf("%s: merchant payable does not match checkout amount", op),
		)
	}

	collectedCashPaymentIntentID := ""
	if collectedCash > 0 {
		if contractRow.FinancialPaymentIntentID == nil || strings.TrimSpace(*contractRow.FinancialPaymentIntentID) == "" {
			return merchantEntitlementRecord{}, cerr.InternalError(
				fmt.Errorf("%s: successful checkout payment intent id is missing", op),
			)
		}
		collectedCashPaymentIntentID = strings.TrimSpace(*contractRow.FinancialPaymentIntentID)
	}

	cashMinor, err := model.AmountMinorInt64(collectedCash)
	if err != nil {
		return merchantEntitlementRecord{}, cerr.InternalError(fmt.Errorf("%s: collected cash amount: %w", op, err))
	}

	receivableMinor, err := model.AmountMinorInt64(receivable)
	if err != nil {
		return merchantEntitlementRecord{}, cerr.InternalError(fmt.Errorf("%s: receivable amount: %w", op, err))
	}

	merchantPayableMinor, err := model.AmountMinorInt64(merchantPayable)
	if err != nil {
		return merchantEntitlementRecord{}, cerr.InternalError(fmt.Errorf("%s: merchant payable amount: %w", op, err))
	}

	result, err := uc.checkoutPaymentClient.RecordMerchantEntitlement(ctx, extcheckoutpayment.RecordMerchantEntitlementInput{
		SourceType:                        financialSourceType,
		SourceID:                          contractRow.ID.String(),
		SourceVersion:                     1,
		MerchantID:                        merchant.MerchantID.String(),
		CustomerSubjectID:                 customer.IdentitySubjectID.String(),
		Currency:                          string(contractRow.Currency),
		CollectedCashAmountMinor:          cashMinor,
		ReceivableAmountMinor:             receivableMinor,
		MerchantPayableAmountMinor:        merchantPayableMinor,
		CollectedCashPaymentIntentID:      collectedCashPaymentIntentID,
		MerchantTransactionFeeAmountMinor: 0,
	})
	if err != nil {
		return merchantEntitlementRecord{}, err
	}

	entitlementID := strings.TrimSpace(result.MerchantEntitlementID)
	if entitlementID == "" || len(entitlementID) > maxFinancialIdentifierLength {
		return merchantEntitlementRecord{}, financialCoreCheckoutPaymentUnavailableError(
			fmt.Errorf("%s: financial core returned invalid merchant entitlement id", op),
		)
	}

	receivableID := strings.TrimSpace(result.ReceivableID)
	if receivableID == "" || len(receivableID) > maxFinancialIdentifierLength {
		return merchantEntitlementRecord{}, financialCoreCheckoutPaymentUnavailableError(
			fmt.Errorf("%s: financial core returned invalid receivable id", op),
		)
	}

	return merchantEntitlementRecord{
		MerchantEntitlementID: entitlementID,
		ReceivableID:          receivableID,
	}, nil
}

func contractStatusAllowed(status model.ContractStatus, allowed []model.ContractStatus) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func contractHasFinancialRecognition(contractRow model.Contract) bool {
	return contractRow.FinancialMerchantEntitlementID != nil &&
		strings.TrimSpace(*contractRow.FinancialMerchantEntitlementID) != "" &&
		contractRow.FinancialReceivableID != nil &&
		strings.TrimSpace(*contractRow.FinancialReceivableID) != ""
}
