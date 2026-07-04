package decisionrp

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core decision repository contract.
var _ repo.Decision = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the decision repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL decision repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.DecisionConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.DecisionTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core decision connection queryer contract.
var _ repo.DecisionConnQueryer = (*connQueryer)(nil)

// GetByID returns the decision identified by id.
func (q connQueryer) GetByID(ctx context.Context, id model.DecisionID) (model.Decision, error) {
	return GetByID(ctx, q.Conn, id)
}

// ListByTarget returns decisions recorded for a target entity.
func (q connQueryer) ListByTarget(ctx context.Context, targetType model.DecisionTargetType, targetID uuid.UUID) ([]model.Decision, error) {
	return ListByTarget(ctx, q.Conn, targetType, targetID)
}

// GetLatestByTargetAndType returns the newest decision of type decisionType for a target.
func (q connQueryer) GetLatestByTargetAndType(ctx context.Context, targetType model.DecisionTargetType, targetID uuid.UUID, decisionType model.DecisionType) (model.Decision, error) {
	return GetLatestByTargetAndType(ctx, q.Conn, targetType, targetID, decisionType)
}

// ListByPriorDecisionID returns decisions that override, renew, or correct a prior decision.
func (q connQueryer) ListByPriorDecisionID(ctx context.Context, priorDecisionID model.DecisionID) ([]model.Decision, error) {
	return ListByPriorDecisionID(ctx, q.Conn, priorDecisionID)
}

// GetByIdempotencyKey returns the decision recorded for idempotencyKey.
func (q connQueryer) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (model.Decision, error) {
	return GetByIdempotencyKey(ctx, q.Conn, idempotencyKey)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core decision transaction queryer contract.
var _ repo.DecisionTxQueryer = (*txQueryer)(nil)

// GetByID returns the decision identified by id.
func (q txQueryer) GetByID(ctx context.Context, id model.DecisionID) (model.Decision, error) {
	return GetByID(ctx, q.Tx, id)
}

// ListByTarget returns decisions recorded for a target entity.
func (q txQueryer) ListByTarget(ctx context.Context, targetType model.DecisionTargetType, targetID uuid.UUID) ([]model.Decision, error) {
	return ListByTarget(ctx, q.Tx, targetType, targetID)
}

// GetLatestByTargetAndType returns the newest decision of type decisionType for a target.
func (q txQueryer) GetLatestByTargetAndType(ctx context.Context, targetType model.DecisionTargetType, targetID uuid.UUID, decisionType model.DecisionType) (model.Decision, error) {
	return GetLatestByTargetAndType(ctx, q.Tx, targetType, targetID, decisionType)
}

// ListByPriorDecisionID returns decisions that override, renew, or correct a prior decision.
func (q txQueryer) ListByPriorDecisionID(ctx context.Context, priorDecisionID model.DecisionID) ([]model.Decision, error) {
	return ListByPriorDecisionID(ctx, q.Tx, priorDecisionID)
}

// GetByIdempotencyKey returns the decision recorded for idempotencyKey.
func (q txQueryer) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (model.Decision, error) {
	return GetByIdempotencyKey(ctx, q.Tx, idempotencyKey)
}

func (q txQueryer) LockLiveCheckoutDecisionByID(
	ctx context.Context,
	id model.DecisionID,
	now time.Time,
) (model.Decision, error) {
	return LockLiveCheckoutDecisionByID(ctx, q.Tx, id, now)
}

// Create stores a new decision audit envelope inside the current transaction.
func (q txQueryer) Create(
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
	priorDecisionID *model.DecisionID,
	reviewableUntil *time.Time,
	expiresAt *time.Time,
	decidedAt time.Time,
	createdAt time.Time,
	idempotencyKey string,
	idempotencyFingerprint string,
) (model.Decision, error) {
	return Create(
		ctx,
		q.Tx,
		decisionType,
		targetType,
		targetID,
		outcome,
		reasonCodes,
		policyID,
		policyVersion,
		jurisdiction,
		actorType,
		actorID,
		priorDecisionID,
		reviewableUntil,
		expiresAt,
		decidedAt,
		createdAt,
		idempotencyKey,
		idempotencyFingerprint,
	)
}
