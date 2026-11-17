package v3

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildInstallmentSchedule produces exactly one deferred principal payment.
// The offer date is provisional; contract activation re-anchors the due date.
func (p policyV3) BuildInstallmentSchedule(input checkout.ScheduleInput) (checkout.ScheduleResult, error) {
	const op = "v3.policyV3.BuildInstallmentSchedule"
	if input.InstallmentCount != 1 || input.UpfrontAmountMinor != 0 {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: monthly credit requires one installment and no upfront cash", op)
	}
	if err := model.ValidatePositiveAmountMinor(input.FinancedAmountMinor); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: financed amount: %w", op, err)
	}
	dueAt, err := checkout.DueAtForInstallmentOrder(p.Terms(), input.Now, 1)
	if err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: compute due date: %w", op, err)
	}
	result := checkout.ScheduleResult{
		Anchor: input.Now,
		Installments: []checkout.InstallmentDraft{{
			Order: 1,
			PrincipalAmountMinor: input.FinancedAmountMinor,
			UpfrontAmountMinor: 0,
			BaseFeeAmountMinor: 0,
			TotalAmountMinor: input.FinancedAmountMinor,
			DueAt: dueAt,
		}},
	}
	if err := checkout.ValidateGeneratedSchedule(p.Terms(), result, input.FinancedAmountMinor, 0); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: validate schedule: %w", op, err)
	}
	return result, nil
}
