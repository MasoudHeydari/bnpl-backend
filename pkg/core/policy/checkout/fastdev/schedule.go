//go:build development

package fastdev

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
)

// EvaluateEligibility reuses v1 eligibility rules; only repayment cadence differs.
func (policyFastDev) EvaluateEligibility(input checkout.EligibilityInput) checkout.EligibilityResult {
	return checkoutv1.New().EvaluateEligibility(input)
}

// EvaluateFinancing reuses v1 financing/allocation math.
func (policyFastDev) EvaluateFinancing(input checkout.FinancingInput) checkout.FinancingResult {
	return checkoutv1.New().EvaluateFinancing(input)
}

// BuildContractPlan reuses v1 activation economics (initial payment required).
func (policyFastDev) BuildContractPlan(input checkout.ContractPlanInput) (checkout.ContractPlan, error) {
	return checkoutv1.New().BuildContractPlan(input)
}

// BuildInstallmentSchedule mirrors v1 monetary allocation while applying this
// policy's five calendar-day repayment cadence from Terms().
func (p policyFastDev) BuildInstallmentSchedule(input checkout.ScheduleInput) (checkout.ScheduleResult, error) {
	const op = "fastdev.policyFastDev.BuildInstallmentSchedule"

	if input.InstallmentCount <= 0 {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: installment count must be positive", op)
	}

	if err := model.ValidatePositiveAmountMinor(input.FinancedAmountMinor); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: financed amount: %w", op, err)
	}

	if err := model.ValidateNonNegativeAmountMinor(input.UpfrontAmountMinor); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: upfront amount: %w", op, err)
	}

	terms := p.Terms()
	if input.InstallmentCount != terms.InstallmentCount {
		return checkout.ScheduleResult{}, fmt.Errorf(
			"%s: installment count %d is not allowed by checkout policy %d",
			op,
			input.InstallmentCount,
			terms.InstallmentCount,
		)
	}

	basePrincipal, remainder := splitPrincipal(input.FinancedAmountMinor, input.InstallmentCount)
	drafts := make([]checkout.InstallmentDraft, 0, input.InstallmentCount)

	for order := 1; order <= input.InstallmentCount; order++ {
		principal := basePrincipal
		if order == 1 {
			var err error
			principal, err = model.AddAmountMinor(principal, remainder)
			if err != nil {
				return checkout.ScheduleResult{}, fmt.Errorf("%s: first installment principal: %w", op, err)
			}
		}

		upfront := uint64(0)
		if order == 1 {
			upfront = input.UpfrontAmountMinor
		}

		total, err := model.SumAmountMinor(principal, upfront, 0)
		if err != nil {
			return checkout.ScheduleResult{}, fmt.Errorf("%s: installment %d total: %w", op, order, err)
		}

		dueAt, err := checkout.DueAtForInstallmentOrder(terms, input.Now, order)
		if err != nil {
			return checkout.ScheduleResult{}, fmt.Errorf("%s: installment %d due_at: %w", op, order, err)
		}

		drafts = append(drafts, checkout.InstallmentDraft{
			Order:                order,
			PrincipalAmountMinor: principal,
			UpfrontAmountMinor:   upfront,
			BaseFeeAmountMinor:   0,
			TotalAmountMinor:     total,
			DueAt:                dueAt,
		})
	}

	result := checkout.ScheduleResult{Anchor: input.Now, Installments: drafts}
	if err := checkout.ValidateGeneratedSchedule(terms, result, input.FinancedAmountMinor, input.UpfrontAmountMinor); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: %w", op, err)
	}

	return result, nil
}

func splitPrincipal(financedAmountMinor uint64, installmentCount int) (basePrincipal, remainder uint64) {
	count := uint64(installmentCount)
	basePrincipal = financedAmountMinor / count
	remainder = financedAmountMinor % count
	return basePrincipal, remainder
}
