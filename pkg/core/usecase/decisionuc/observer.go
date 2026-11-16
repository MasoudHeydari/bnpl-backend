package decisionuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records operational flow checkpoints for checkout decision
// evaluation and offer revalidation. Implementations must not modify domain
// state or return errors that affect business execution.
//
// Success checkpoints are emitted only after the corresponding database
// transaction committed. Idempotent replays are reported through
// DecisionReplayed and never as a new approval or denial.
type Observer interface {
	// EvaluationStarted records the start of a checkout decision evaluation.
	EvaluationStarted(ctx context.Context, externalSubjectID model.ExternalSubjectID)

	// AvailableCreditChecked records the evaluation-time available credit
	// snapshot of a newly persisted decision.
	AvailableCreditChecked(
		ctx context.Context,
		decisionID model.DecisionID,
		availableAmountMinor uint64,
		currency model.Currency,
	)

	// DecisionApproved records a committed fully approved decision.
	DecisionApproved(ctx context.Context, decisionID model.DecisionID)

	// DecisionPartiallyApproved records a committed conditional decision.
	DecisionPartiallyApproved(ctx context.Context, decisionID model.DecisionID)

	// DecisionDenied records a committed denied decision with its first reason code.
	DecisionDenied(ctx context.Context, decisionID model.DecisionID, reasonCode string)

	// DecisionPersisted records a newly committed decision of any outcome.
	DecisionPersisted(ctx context.Context, decisionID model.DecisionID, outcome model.DecisionOutcome)

	// DecisionReplayed records an idempotent replay of an existing decision.
	DecisionReplayed(ctx context.Context, decisionID model.DecisionID, outcome model.DecisionOutcome)

	// OfferExpired records that a persisted offer is past its expiry.
	OfferExpired(ctx context.Context, decisionID model.DecisionID)

	// OfferInvalidated records that a persisted offer can no longer be honored
	// because current state no longer supports its terms.
	OfferInvalidated(ctx context.Context, decisionID model.DecisionID, reasonCode string)
}
