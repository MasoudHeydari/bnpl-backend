package v1

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildInstallmentSchedule builds the offered-installment schedule for a
// financed checkout allocation: principal allocation (equal split, remainder
// on the first installment), upfront placement, and due dates.
//
// Installment #1 is the checkout activation payment due within
// FirstInstallmentPaymentWindow of the supplied Now (decision time for the
// provisional offer). Recurring installments #2..n are spaced by the policy
// cadence from the same Now at decision time; when ActivationMode requires
// ScheduleFromContractActivation, contract activation rebuilds those recurring
// dues from the authoritative activation instant without changing monetary
// allocation or installment #1's checkout payment deadline.
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

		baseFee := uint64(0)
		total, err := model.SumAmountMinor(principal, upfront, baseFee)
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
			BaseFeeAmountMinor:   baseFee,
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
