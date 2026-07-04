package contractuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// finalizeOfferedInstallmentDueDatesAtActivation rebuilds contractual due dates
// from the authoritative activation instant when the selected Checkout Policy
// requires ScheduleFromContractActivation.
//
// Monetary principal/upfront/totals are immutable snapshots from Decision time
// and must match the rebuilt drafts. For ActivationAfterInitialPayment, the
// checkout-payment installment (#1) keeps its original decision-anchored due
// date; only recurring installments are rewritten. Policies that pay no initial
// installment rewrite the entire recurring schedule.
//
// Callers must invoke this while installments are still offered and before
// MarkScheduledOrDue / MarkPaid transitions.
func finalizeOfferedInstallmentDueDatesAtActivation(
	ctx context.Context,
	installmentQ repo.InstallmentTxQueryer,
	selected checkout.Policy,
	contractRow model.Contract,
	installments []model.Installment,
	plan checkout.ContractPlan,
	activationAt time.Time,
	updatedAt time.Time,
) ([]model.Installment, error) {
	const op = "contractuc.finalizeOfferedInstallmentDueDatesAtActivation"

	if selected.Terms().ScheduleAnchor != checkout.ScheduleFromContractActivation {
		return installments, nil
	}

	schedule, err := selected.BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 activationAt,
		FinancedAmountMinor: contractRow.ApprovedFinancedAmountMinor,
		UpfrontAmountMinor:  contractRow.UpfrontAmountMinor,
		InstallmentCount:    contractRow.InstallmentCount,
	})
	if err != nil {
		return nil, cerr.InternalError(fmt.Errorf("%s: finalize installment schedule: %w", op, err))
	}

	if len(schedule.Installments) != len(installments) {
		return nil, cerr.InternalError(fmt.Errorf("%s: finalized installment count mismatch", op))
	}

	byOrder := make(map[int]checkout.InstallmentDraft, len(schedule.Installments))
	for _, draft := range schedule.Installments {
		byOrder[draft.Order] = draft
	}

	updated := make([]model.Installment, 0, len(installments))
	for _, installment := range installments {
		draft, exists := byOrder[installment.InstallmentOrder]
		if !exists ||
			draft.PrincipalAmountMinor != installment.PrincipalAmountMinor ||
			draft.UpfrontAmountMinor != installment.UpfrontAmountMinor ||
			draft.TotalAmountMinor != installment.TotalAmountMinor {
			return nil, cerr.InternalError(fmt.Errorf("%s: finalized installment economics differ from decision snapshot", op))
		}

		// The normal-customer checkout payment deadline stays decision-anchored.
		// Recurring dues alone move to the activation cadence.
		if plan.FirstInstallmentPaidOnActivation && installment.InstallmentOrder == 1 {
			updated = append(updated, installment)
			continue
		}

		next, updateErr := installmentQ.UpdateOfferedDueAt(ctx, installment.ID, draft.DueAt, updatedAt)
		if updateErr != nil {
			return nil, updateErr
		}

		updated = append(updated, next)
	}

	return updated, nil
}
