package v2

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildInstallmentSchedule builds v2 installments with no upfront amount. The
// supplied Now is treated as a provisional anchor at decision time and as the
// authoritative activation anchor when the schedule is finalized for contract
// creation. With the v2 terms, installment n is due n Jalali months after that
// anchor (n = 1..InstallmentCount).
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

	basePrincipal, remainder := splitPrincipal(input.FinancedAmountMinor, input.InstallmentCount)
	drafts := make([]checkout.InstallmentDraft, 0, input.InstallmentCount)
	firstOffset := p.Terms().FirstInstallmentDueAfterMonths

	for order := 1; order <= input.InstallmentCount; order++ {
		principal := basePrincipal
		if order == 1 {
			principal += remainder
		}

		dueAt := jalali.AddMonthsClamped(input.Now, firstOffset+order-1)
		drafts = append(drafts, checkout.InstallmentDraft{
			Order:                order,
			PrincipalAmountMinor: principal,
			UpfrontAmountMinor:   0,
			BaseFeeAmountMinor:   0,
			TotalAmountMinor:     principal,
			DueAt:                dueAt,
		})
	}

	return checkout.ScheduleResult{Installments: drafts}, nil
}
