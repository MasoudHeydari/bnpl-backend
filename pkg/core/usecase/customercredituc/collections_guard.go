package customercredituc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// collectionsBlocksNewCredit reports whether the newest collections_default
// restriction Decision for the customer currently blocks new credit usage.
//
// Other restriction policies are ignored so releasing collections does not
// confuse independent fraud/risk restriction Decisions, and vice versa.
func collectionsBlocksNewCredit(
	ctx context.Context,
	decisionQ repo.DecisionQueryer,
	customerID model.CustomerID,
) (bool, error) {
	const op = "customercredituc.collectionsBlocksNewCredit"

	decisions, err := decisionQ.ListByTarget(
		ctx,
		model.CustomerProfileDecisionTargetType,
		customerID.UUID(),
	)
	if err != nil {
		return false, fmt.Errorf("%s: list decisions: %w", op, err)
	}

	for _, decision := range decisions {
		if decision.Type != model.RestrictionDecisionType {
			continue
		}

		if decision.PolicyID != collectionsv1.PolicyID {
			continue
		}

		return decision.Outcome == model.RestrictedDecisionOutcome, nil
	}

	return false, nil
}
