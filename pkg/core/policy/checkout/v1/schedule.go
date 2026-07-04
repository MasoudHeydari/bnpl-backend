package v1

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildInstallmentSchedule builds the offered-installment schedule for a
// financed checkout allocation: principal allocation (equal split, remainder
// on the first installment), upfront placement, and due dates.
//
// The first installment is payable within firstInstallmentPaymentWindow of now.
// Later installments follow Jalali (Persian) calendar-month tenor from now,
// preserving the Jalali day-of-month when possible. Shorter target months clamp
// to the last day of that Jalali month (for example Farvardin 31 -> Mehr 30).
// Tenor is computed on the Jalali calendar so schedules stay on the same
// Persian day across both 31-day and 30/29-day halves of the year.
func (p policyV1) BuildInstallmentSchedule(input checkout.ScheduleInput) (checkout.ScheduleResult, error) {
	const op = "v1.policyV1.BuildInstallmentSchedule"

	if input.InstallmentCount <= 0 {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: installment count must be positive", op)
	}

	if err := model.ValidatePositiveAmountMinor(input.FinancedAmountMinor); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: financed amount: %w", op, err)
	}

	if err := model.ValidateNonNegativeAmountMinor(input.UpfrontAmountMinor); err != nil {
		return checkout.ScheduleResult{}, fmt.Errorf("%s: upfront amount: %w", op, err)
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

		baseFee := uint64(0)
		total, err := model.SumAmountMinor(principal, upfront, baseFee)
		if err != nil {
			return checkout.ScheduleResult{}, fmt.Errorf("%s: installment %d total: %w", op, order, err)
		}

		dueAt := jalali.AddMonthsClamped(input.Now, order-1)
		if order == 1 {
			dueAt = input.Now.Add(p.Terms().FirstInstallmentPaymentWindow)
		}

		drafts = append(drafts, checkout.InstallmentDraft{
			Order:                order,
			PrincipalAmountMinor: principal,
			UpfrontAmountMinor:   upfront,
			BaseFeeAmountMinor:   baseFee,
			TotalAmountMinor:     total,
			DueAt:                dueAt,
		})
	}

	return checkout.ScheduleResult{Installments: drafts}, nil
}
