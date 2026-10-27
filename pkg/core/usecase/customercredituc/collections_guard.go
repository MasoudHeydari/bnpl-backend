package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// newCreditUsageBlocked reports whether the customer currently cannot use new
// credit. Active CustomerRestriction rows are the source of truth for applied
// restrictions. Live contractual default eligibility is also checked so the
// summary does not advertise usable credit during the task-delay gap before a
// restriction is materialized.
func newCreditUsageBlocked(
	ctx context.Context,
	restrictionQ repo.CustomerRestrictionQueryer,
	installmentQ repo.InstallmentQueryer,
	episodeQ repo.DefaultEpisodeQueryer,
	customerID model.CustomerID,
	now time.Time,
) (bool, error) {
	const op = "customercredituc.newCreditUsageBlocked"

	activeRestrictions, err := restrictionQ.ListActiveByCustomerID(ctx, customerID)
	if err != nil {
		return false, fmt.Errorf("%s: list active restrictions: %w", op, err)
	}

	for _, restriction := range activeRestrictions {
		if restriction.RestrictionType == model.NewCreditUsageCustomerRestrictionType {
			return true, nil
		}
	}

	_, episodeErr := episodeQ.GetActiveByCustomerID(ctx, customerID)
	if episodeErr == nil {
		return true, nil
	}

	if !cerr.IsCode(episodeErr, cerr.CodeResourceNotFound) {
		return false, fmt.Errorf("%s: load active episode: %w", op, episodeErr)
	}

	obligations, err := installmentQ.ListCollectionsObligationsByCustomerID(ctx, customerID)
	if err != nil {
		return false, fmt.Errorf("%s: list obligations: %w", op, err)
	}

	facts := make([]collectionsv1.ObligationFact, 0, len(obligations))
	for _, row := range obligations {
		facts = append(facts, collectionsv1.ObligationFact{
			InstallmentID:             row.Installment.ID,
			ContractID:                row.Installment.ContractID,
			DueAt:                     row.Installment.DueAt,
			GracePeriodDays:           row.Installment.GracePeriodDays,
			Status:                    row.Installment.Status,
			IsFullySettled:            false,
			HasUnresolvedPayment:      row.HasUnresolvedPayment,
			IsContractuallyApplicable: true,
		})
	}

	evaluation := collectionsv1.New().Evaluate(facts, now, false)
	return evaluation.IsDefaultEligible, nil
}
