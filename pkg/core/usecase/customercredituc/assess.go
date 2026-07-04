package customercredituc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// AssessInitialLimit evaluates whether a customer should receive an initial
// credit limit using the policy currently assigned to the Identity subject.
func (uc *UseCase) AssessInitialLimit(
	ctx context.Context,
	input InitialLimitAssessmentInput,
) (InitialLimitAssessmentResult, error) {
	now := input.RequestedAt.UTC()
	var selected initialcreditlimit.Policy
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var err error
		policyAssignmentQ := uc.policyAssignment.Conn(conn)
		selected, err = uc.resolveInitialLimitPolicy(ctx, policyAssignmentQ, input.ExternalCustomerID, now)
		return err
	})
	if err != nil {
		return InitialLimitAssessmentResult{}, err
	}

	return uc.assessInitialLimitWithPolicy(ctx, input, selected)
}

func (uc *UseCase) assessInitialLimitWithPolicy(
	ctx context.Context,
	input InitialLimitAssessmentInput,
	selected initialcreditlimit.Policy,
) (InitialLimitAssessmentResult, error) {
	_, err := uc.creditAssessmentClient.FetchSignals(
		ctx,
		creditassessment.FetchSignalsInput{
			CustomerID:         input.CustomerID,
			ExternalCustomerID: input.ExternalCustomerID,
			Jurisdiction:       input.Jurisdiction,
			RequestedAt:        input.RequestedAt,
		},
	)
	if err != nil {
		return InitialLimitAssessmentResult{}, err
	}

	terms := selected.Terms()
	amount := terms.DefaultAmountMinor
	if amount > terms.MaxAmountMinor {
		amount = terms.MaxAmountMinor
	}

	return InitialLimitAssessmentResult{
		Outcome:       model.LimitSetDecisionOutcome,
		AmountMinor:   amount,
		Currency:      terms.Currency,
		ValidFor:      terms.ValidFor,
		PolicyID:      selected.Ref().ID,
		PolicyVersion: selected.Ref().Version,
		ReasonCodes:   []string{terms.ApprovalReasonCode},
	}, nil
}
