package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// DecisionQueryer defines read-only decision operations.
type DecisionQueryer interface {
	// GetByID returns the decision identified by id.
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.Decision, error)

	// ListByTarget returns decisions recorded for a target entity.
	ListByTarget(
		ctx context.Context,
		targetType model.DecisionTargetType,
		targetID uuid.UUID,
	) ([]model.Decision, error)

	// GetLatestByTargetAndType returns the newest decision of type decisionType for a target.
	GetLatestByTargetAndType(
		ctx context.Context,
		targetType model.DecisionTargetType,
		targetID uuid.UUID,
		decisionType model.DecisionType,
	) (model.Decision, error)

	// ListByPriorDecisionID returns decisions that override, renew, or correct a prior decision.
	ListByPriorDecisionID(
		ctx context.Context,
		priorDecisionID uuid.UUID,
	) ([]model.Decision, error)

	// GetByIdempotencyKey returns the decision recorded for idempotencyKey.
	GetByIdempotencyKey(
		ctx context.Context,
		idempotencyKey string,
	) (model.Decision, error)
}

// DecisionConnQueryer exposes decision operations on a connection.
type DecisionConnQueryer interface {
	DecisionQueryer
}

// DecisionTxQueryer exposes transaction-scoped decision operations.
type DecisionTxQueryer interface {
	DecisionQueryer

	// Create stores a new decision audit envelope inside the current transaction.
	Create(
		ctx context.Context,
		decisionType model.DecisionType,
		targetType model.DecisionTargetType,
		targetID uuid.UUID,
		outcome model.DecisionOutcome,
		reasonCodes []string,
		policyID string,
		policyVersion string,
		jurisdiction model.Jurisdiction,
		actorType model.DecisionActorType,
		actorID *uuid.UUID,
		priorDecisionID *uuid.UUID,
		reviewableUntil *time.Time,
		expiresAt *time.Time,
		decidedAt time.Time,
		createdAt time.Time,
		idempotencyKey string,
		idempotencyFingerprint string,
	) (model.Decision, error)
}

// Decision persists immutable governed decision records.
type Decision interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) DecisionConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) DecisionTxQueryer
}
