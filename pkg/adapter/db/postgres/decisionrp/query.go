package decisionrp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const decisionColumns = `
	id,
	type,
	target_type,
	target_id,
	outcome,
	reason_codes,
	policy_id,
	policy_version,
	jurisdiction,
	actor_type,
	actor_id,
	prior_decision_id,
	reviewable_until,
	expires_at,
	decided_at,
	created_at,
	idempotency_key,
	idempotency_fingerprint
`

// Create inserts a new immutable decision record and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
	priorDecisionID *model.DecisionID,
	reviewableUntil *time.Time,
	expiresAt *time.Time,
	decidedAt time.Time,
	createdAt time.Time,
	idempotencyKey string,
	idempotencyFingerprint string,
) (model.Decision, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"decided_at": decidedAt,
		"created_at": createdAt,
	}); err != nil {
		return model.Decision{}, err
	}

	const query = `
INSERT INTO decisions (
	type,
	target_type,
	target_id,
	outcome,
	reason_codes,
	policy_id,
	policy_version,
	jurisdiction,
	actor_type,
	actor_id,
	prior_decision_id,
	reviewable_until,
	expires_at,
	decided_at,
	created_at,
	idempotency_key,
	idempotency_fingerprint
)
VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13, $14, $15, $16, $17
)
RETURNING` + decisionColumns

	return postgres.QueryOne[model.Decision](
		ctx,
		q,
		"create decision",
		query,
		decisionType,
		targetType,
		targetID,
		outcome,
		reasonCodes,
		policyID,
		policyVersion,
		jurisdiction,
		actorType,
		postgres.Nullable(actorID),
		postgres.Nullable(priorDecisionID),
		postgres.Nullable(reviewableUntil),
		postgres.Nullable(expiresAt),
		decidedAt,
		createdAt,
		idempotencyKey,
		idempotencyFingerprint,
	)
}

// GetByID returns a decision by id.
func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id model.DecisionID) (model.Decision, error) {
	const query = `SELECT` + decisionColumns + `FROM decisions WHERE id = $1`

	return postgres.QueryOne[model.Decision](ctx, q, "get decision by id", query, id)
}

// GetByIdempotencyKey returns a decision by idempotency key.
func GetByIdempotencyKey[Q postgres.Queryer](ctx context.Context, q Q, idempotencyKey string) (model.Decision, error) {
	const query = `SELECT` + decisionColumns + `FROM decisions WHERE idempotency_key = $1`

	return postgres.QueryOne[model.Decision](ctx, q, "get decision by idempotency key", query, idempotencyKey)
}

// LockLiveCheckoutDecisionByID locks an approved or conditional checkout decision
// that has not expired.
func LockLiveCheckoutDecisionByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.DecisionID,
	now time.Time,
) (model.Decision, error) {
	const query = `SELECT` + decisionColumns + `
FROM decisions
WHERE id = $1
  AND outcome IN ($2, $3)
  AND expires_at IS NOT NULL
  AND expires_at > $4
FOR UPDATE
`

	return postgres.QueryOne[model.Decision](
		ctx,
		q,
		"lock live checkout decision by id",
		query,
		id,
		model.ApprovedDecisionOutcome,
		model.ConditionalDecisionOutcome,
		now,
	)
}

// ListByTarget returns decisions recorded for targetType and targetID.
func ListByTarget[Q postgres.Queryer](ctx context.Context, q Q, targetType model.DecisionTargetType, targetID uuid.UUID) ([]model.Decision, error) {
	const query = `SELECT` + decisionColumns + `
FROM decisions
WHERE target_type = $1
  AND target_id = $2
ORDER BY decided_at DESC, created_at DESC
`

	return getMany(ctx, q, query, []any{targetType, targetID}, "list decisions by target")
}

// GetLatestByTargetAndType returns the newest decision of decisionType for a target.
func GetLatestByTargetAndType[Q postgres.Queryer](ctx context.Context, q Q, targetType model.DecisionTargetType, targetID uuid.UUID, decisionType model.DecisionType) (model.Decision, error) {
	const query = `SELECT` + decisionColumns + `
FROM decisions
WHERE target_type = $1
  AND target_id = $2
  AND type = $3
ORDER BY decided_at DESC, created_at DESC
LIMIT 1
`

	return postgres.QueryOne[model.Decision](ctx, q, "get latest decision by target and type", query, targetType, targetID, decisionType)
}

// ListByPriorDecisionID returns decisions that reference a prior decision.
func ListByPriorDecisionID[Q postgres.Queryer](ctx context.Context, q Q, priorDecisionID model.DecisionID) ([]model.Decision, error) {
	const query = `SELECT` + decisionColumns + `
FROM decisions
WHERE prior_decision_id = $1
ORDER BY decided_at DESC, created_at DESC
`

	return getMany(ctx, q, query, []any{priorDecisionID}, "list decisions by prior decision id")
}

func getMany[Q postgres.Queryer](ctx context.Context, q Q, query string, args []any, operation string) ([]model.Decision, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, postgres.MapError(err))
	}

	defer rows.Close()

	decisions, err := postgres.ValuesInto[model.Decision](postgres.MustRows(rows))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}

	return decisions, nil
}
