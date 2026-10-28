package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
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
	selected collections.Policy,
) (bool, string, error) {
	const op = "customercredituc.newCreditUsageBlocked"

	activeRestrictions, err := restrictionQ.ListActiveByCustomerID(ctx, customerID)
	if err != nil {
		return false, "", fmt.Errorf("%s: list active restrictions: %w", op, err)
	}

	blocked, other := false, false
	for _, restriction := range activeRestrictions {
		if restriction.RestrictionType == model.NewCreditUsageCustomerRestrictionType {
			blocked = true
			other = other || restriction.Reason != model.CollectionsDefaultCustomerRestrictionReason
		}
	}
	if blocked {
		if other {
			return true, "credit_usage_restricted", nil
		}
		return true, "collections_default", nil
	}

	_, episodeErr := episodeQ.GetActiveByCustomerID(ctx, customerID)
	if episodeErr == nil {
		return true, "collections_default", nil
	}

	if !cerr.IsCode(episodeErr, cerr.CodeResourceNotFound) {
		return false, "", fmt.Errorf("%s: load active episode: %w", op, episodeErr)
	}

	obligations, err := installmentQ.ListCollectionsObligationsByCustomerID(ctx, customerID)
	if err != nil {
		return false, "", fmt.Errorf("%s: list obligations: %w", op, err)
	}

	facts := make([]collections.ObligationFact, 0, len(obligations))
	for _, row := range obligations {
		fact := collections.FactFromUnpaidInstallment(row.Installment, row.HasUnresolvedPayment)
		fact.HasUnsettledCharges = row.HasUnsettledCharges
		fact.HasUnresolvedCancellation = row.HasUnresolvedCancellation
		facts = append(facts, fact)
	}

	evaluation := selected.Evaluate(facts, now, false)
	if evaluation.HasUnresolvedCancellation {
		return true, "credit_usage_restricted", nil
	}
	return evaluation.IsDefaultEligible, "collections_default", nil
}
