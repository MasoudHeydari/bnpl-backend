package v2

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildInstallmentSchedule builds v2 installments with no upfront amount. The
// supplied Now is treated as a provisional anchor at decision time and as the
// authoritative activation anchor when the schedule is finalized for contract
// creation. Each installment n is due after n full repayment-cadence intervals
// from that anchor.
func (p policyV2) BuildInstallmentSchedule(input checkout.ScheduleInput) (checkout.ScheduleResult, error) {
	const op = "v2.policyV2.BuildInstallmentSchedule"

	if input.InstallmentCount <= 0 {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: installment count must be positive", op)
	}

	if err := model.ValidatePositiveAmountMinor(input.FinancedAmountMinor); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: financed amount: %w", op, err)
	}

	if input.UpfrontAmountMinor != 0 {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: v2 does not allow upfront amount", op)
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
			principal += remainder
		}

		dueAt, err := checkout.DueAtForInstallmentOrder(terms, input.Now, order)
		if err != nil {
			return checkout.ScheduleResult{}, fmt.Errorf("%s: installment %d due_at: %w", op, order, err)
		}

		drafts = append(drafts, checkout.InstallmentDraft{
			Order:                order,
			PrincipalAmountMinor: principal,
			UpfrontAmountMinor:   0,
			BaseFeeAmountMinor:   0,
			TotalAmountMinor:     principal,
			DueAt:                dueAt,
		})
	}

	result := checkout.ScheduleResult{Anchor: input.Now, Installments: drafts}
	if err := checkout.ValidateGeneratedSchedule(terms, result, input.FinancedAmountMinor, 0); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: %w", op, err)
	}

	return result, nil
}
