package decisionuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// InstallmentOffer is the persisted installment schedule attached to an
// approved or partially approved checkout decision. For policies anchored to
// contract activation, due dates are provisional until successful activation;
// installment economics remain fixed.
type InstallmentOffer struct {
	// InstallmentCount is the number of installments in the schedule.
	InstallmentCount int
	// PayableNowAmountMinor is the amount payable immediately in minor currency units.
	PayableNowAmountMinor uint64
	// TotalPayableAmountMinor is the total payable amount in minor currency units.
	TotalPayableAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// ExpiresAt is when the offer, session, or limit expires.
	ExpiresAt time.Time
	// Installments lists the installment views or records.
	Installments []model.Installment
	// InstallmentCharges lists the installment charge records.
	InstallmentCharges []model.InstallmentCharge
}

// ProjectCheckoutPresentation rebuilds installment-offer economics and the
// checkout action from persisted allocation and schedule data using the
// Decision's checkout policy. Hosted GET uses this so it never invents
// financed/upfront amounts from installment totals.
func ProjectCheckoutPresentation(
	selectedPolicy checkout.Policy,
	allocation model.CheckoutDecisionAllocation,
	installments []model.Installment,
	charges []model.InstallmentCharge,
) (InstallmentOffer, CheckoutAction, error) {
	return buildCheckoutPresentation(selectedPolicy, allocation, installments, charges)
}

func buildCheckoutPresentation(
	selectedPolicy checkout.Policy,
	allocation model.CheckoutDecisionAllocation,
	installments []model.Installment,
	charges []model.InstallmentCharge,
) (InstallmentOffer, CheckoutAction, error) {
	const op = "decisionuc.buildCheckoutPresentation"

	totalPayable, err := totalPayableAmountMinor(installments)
	if err != nil {
		return InstallmentOffer{}, CheckoutAction{}, err
	}

	first, ok := firstInstallment(installments)
	if !ok {
		return InstallmentOffer{}, CheckoutAction{}, fmt.Errorf("%s: approved offer has no first installment", op)
	}

	plan, err := selectedPolicy.BuildContractPlan(checkout.ContractPlanInput{
		RequestedAmountMinor:                 allocation.RequestedAmountMinor,
		FinancedAmountMinor:                  allocation.ApprovedFinancedAmountMinor,
		UpfrontAmountMinor:                   allocation.UpfrontAmountMinor,
		FirstInstallmentPrincipalAmountMinor: first.PrincipalAmountMinor,
		FirstInstallmentTotalAmountMinor:     first.TotalAmountMinor,
	})
	if err != nil {
		return InstallmentOffer{}, CheckoutAction{}, fmt.Errorf("%s: build contract plan: %w", op, err)
	}

	action, err := checkoutActionFromContractPlan(plan)
	if err != nil {
		return InstallmentOffer{}, CheckoutAction{}, err
	}

	return InstallmentOffer{
		InstallmentCount:        allocation.InstallmentCount,
		PayableNowAmountMinor:   plan.InitialPaymentAmountMinor,
		TotalPayableAmountMinor: totalPayable,
		Currency:                allocation.Currency,
		ExpiresAt:               allocation.ExpiresAt,
		Installments:            installments,
		InstallmentCharges:      charges,
	}, action, nil
}

func firstInstallment(installments []model.Installment) (model.Installment, bool) {
	for _, installment := range installments {
		if installment.InstallmentOrder == 1 {
			return installment, true
		}
	}

	return model.Installment{}, false
}

func totalPayableAmountMinor(installments []model.Installment) (uint64, error) {
	var total uint64
	for _, installment := range installments {
		sum, err := model.AddAmountMinor(total, installment.TotalAmountMinor)
		if err != nil {
			return 0, err
		}

		total = sum
	}

	return total, nil
}

func installmentIDs(installments []model.Installment) []model.InstallmentID {
	if len(installments) == 0 {
		return nil
	}

	ids := make([]model.InstallmentID, len(installments))
	for i := range installments {
		ids[i] = installments[i].ID
	}

	return ids
}

func (uc *UseCase) loadPersistedInstallmentOffer(
	ctx context.Context,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	allocation model.CheckoutDecisionAllocation,
	decision model.Decision,
) (*InstallmentOffer, *CheckoutAction, error) {
	installments, err := installmentQ.ListByDecisionID(ctx, decision.ID)
	if err != nil {
		return nil, nil, err
	}

	if len(installments) == 0 {
		return nil, nil, nil
	}

	charges, err := installmentChargeQ.ListByInstallmentIDs(ctx, installmentIDs(installments))
	if err != nil {
		return nil, nil, err
	}

	if err = validatePersistedOffer(allocation, decision, installments, charges); err != nil {
		return nil, nil, err
	}

	selectedPolicy, err := uc.resolveCheckoutPolicyRef(policy.Ref{ID: decision.PolicyID, Version: decision.PolicyVersion})
	if err != nil {
		return nil, nil, err
	}

	offer, action, err := buildCheckoutPresentation(selectedPolicy, allocation, installments, charges)
	if err != nil {
		return nil, nil, err
	}

	return &offer, &action, nil
}

// createApprovedDecisionInstallments persists offered installments and charges
// for an approved or partially approved checkout decision inside the caller's
// transaction. Calculation uses the same checkout policy schedule builder that
// previously lived only in a standalone installment-offer use case.
func (uc *UseCase) createApprovedDecisionInstallments(
	ctx context.Context,
	selectedPolicy checkout.Policy,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	allocation model.CheckoutDecisionAllocation,
	decision model.Decision,
	now time.Time,
) (InstallmentOffer, CheckoutAction, error) {
	scheduleResult, err := selectedPolicy.BuildInstallmentSchedule(
		checkout.ScheduleInput{
			Now:                 now,
			FinancedAmountMinor: allocation.ApprovedFinancedAmountMinor,
			UpfrontAmountMinor:  allocation.UpfrontAmountMinor,
			InstallmentCount:    allocation.InstallmentCount,
		},
	)
	if err != nil {
		return InstallmentOffer{}, CheckoutAction{}, err
	}

	if err = validateOfferedInstallmentSchedule(
		allocation.InstallmentCount,
		allocation.ApprovedFinancedAmountMinor,
		allocation.Currency,
		scheduleResult.Installments,
	); err != nil {
		return InstallmentOffer{}, CheckoutAction{}, err
	}

	installments, charges, err := uc.persistOfferedInstallments(
		ctx,
		installmentQ,
		installmentChargeQ,
		allocation,
		decision,
		selectedPolicy,
		scheduleResult.Installments,
		now,
	)
	if err != nil {
		return InstallmentOffer{}, CheckoutAction{}, err
	}

	return buildCheckoutPresentation(selectedPolicy, allocation, installments, charges)
}

func (uc *UseCase) persistOfferedInstallments(
	ctx context.Context,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	allocation model.CheckoutDecisionAllocation,
	decision model.Decision,
	selectedPolicy checkout.Policy,
	drafts []checkout.InstallmentDraft,
	now time.Time,
) ([]model.Installment, []model.InstallmentCharge, error) {
	installments := make([]model.Installment, 0, len(drafts))
	allCharges := make([]model.InstallmentCharge, 0)

	for _, draft := range drafts {
		row, err := installmentQ.Create(
			ctx,
			decision.ID,
			nil,
			allocation.CustomerID,
			allocation.MerchantProfileID,
			allocation.MerchantBranchProfileID,
			draft.Order,
			allocation.Currency,
			draft.PrincipalAmountMinor,
			draft.UpfrontAmountMinor,
			draft.BaseFeeAmountMinor,
			draft.TotalAmountMinor,
			draft.DueAt,
			selectedPolicy.Terms().GracePeriodDays,
			model.OfferedInstallmentStatus,
			0,
			nil,
			now,
			now,
		)
		if err != nil {
			return nil, nil, err
		}

		installments = append(installments, row)

		if draft.BaseFeeAmountMinor == 0 {
			continue
		}

		charge, chargeErr := installmentChargeQ.Create(
			ctx,
			row.ID,
			model.BNPLFeeInstallmentChargeType,
			model.OfferedInstallmentChargeStatus,
			draft.BaseFeeAmountMinor,
			allocation.Currency,
			model.CustomerInstallmentChargePayerType,
			model.PlatformInstallmentChargeRecipientType,
			decision.PolicyID,
			decision.PolicyVersion,
			now,
			&draft.DueAt,
			nil,
			json.RawMessage(`{}`),
			now,
			now,
		)
		if chargeErr != nil {
			return nil, nil, chargeErr
		}

		allCharges = append(allCharges, charge)
	}

	return installments, allCharges, nil
}

func outcomeCreatesInstallmentOffer(outcome model.DecisionOutcome) bool {
	switch outcome {
	case model.ApprovedDecisionOutcome, model.ConditionalDecisionOutcome:
		return true

	default:
		return false
	}
}
