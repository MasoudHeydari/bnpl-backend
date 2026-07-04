package sagarp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core Saga repository contract.
var _ repo.Saga = (*Repo)(nil)

// Repo is the stateless PostgreSQL implementation of the Saga repository.
type Repo struct{}

// New creates a PostgreSQL Saga repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.SagaConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.SagaTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.SagaConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByID(
	ctx context.Context,
	id model.SagaID,
) (model.SagaInstance, error) {
	return GetByID(ctx, q.Conn, id)
}

func (q connQueryer) GetByTypeAndBusinessKey(
	ctx context.Context,
	sagaType model.SagaType,
	businessKey model.SagaBusinessKey,
) (model.SagaInstance, error) {
	return GetByTypeAndBusinessKey(ctx, q.Conn, sagaType, businessKey)
}

func (q connQueryer) ListForwardStepCompletions(
	ctx context.Context,
	sagaID model.SagaID,
) ([]model.SagaForwardStepCompletion, error) {
	return ListForwardStepCompletions(ctx, q.Conn, sagaID)
}

func (q connQueryer) ClaimByID(
	ctx context.Context,
	id model.SagaID,
	workerID string,
	now time.Time,
	leaseDuration time.Duration,
) (model.SagaInstance, error) {
	return ClaimByID(
		ctx,
		q.Conn,
		id,
		workerID,
		now,
		leaseDuration,
	)
}

func (q connQueryer) ClaimDue(
	ctx context.Context,
	workerID string,
	limit int,
	now time.Time,
	leaseDuration time.Duration,
) ([]model.SagaInstance, error) {
	return ClaimDue(
		ctx,
		q.Conn,
		workerID,
		limit,
		now,
		leaseDuration,
	)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.SagaTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(
	ctx context.Context,
	id model.SagaID,
) (model.SagaInstance, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) GetByTypeAndBusinessKey(
	ctx context.Context,
	sagaType model.SagaType,
	businessKey model.SagaBusinessKey,
) (model.SagaInstance, error) {
	return GetByTypeAndBusinessKey(ctx, q.Tx, sagaType, businessKey)
}

func (q txQueryer) ListForwardStepCompletions(
	ctx context.Context,
	sagaID model.SagaID,
) ([]model.SagaForwardStepCompletion, error) {
	return ListForwardStepCompletions(ctx, q.Tx, sagaID)
}

func (q txQueryer) Create(
	ctx context.Context,
	sagaType model.SagaType,
	definitionVersion model.SagaDefinitionVersion,
	businessKey model.SagaBusinessKey,
	subjectType *string,
	subjectID *string,
	phase model.SagaPhase,
	status model.SagaStatus,
	currentStep model.SagaStep,
	nextAttemptAt *time.Time,
	correlationID *string,
	causationID *string,
	createdAt time.Time,
	updatedAt time.Time,
) (model.SagaInstance, error) {
	return Create(
		ctx,
		q.Tx,
		sagaType,
		definitionVersion,
		businessKey,
		subjectType,
		subjectID,
		phase,
		status,
		currentStep,
		nextAttemptAt,
		correlationID,
		causationID,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) LockByID(
	ctx context.Context,
	id model.SagaID,
) (model.SagaInstance, error) {
	return LockByID(ctx, q.Tx, id)
}

func (q txQueryer) Update(
	ctx context.Context,
	id model.SagaID,
	expectedVersion int64,
	phase model.SagaPhase,
	status model.SagaStatus,
	currentStep model.SagaStep,
	stepAttemptCount int,
	nextAttemptAt *time.Time,
	lastError *string,
	finishedAt *time.Time,
	updatedAt time.Time,
) (model.SagaInstance, error) {
	return Update(
		ctx,
		q.Tx,
		id,
		expectedVersion,
		phase,
		status,
		currentStep,
		stepAttemptCount,
		nextAttemptAt,
		lastError,
		finishedAt,
		updatedAt,
	)
}

func (q txQueryer) RecordForwardStepCompletion(
	ctx context.Context,
	sagaID model.SagaID,
	step model.SagaStep,
	attemptCount int,
	completedAt time.Time,
) (model.SagaForwardStepCompletion, error) {
	return RecordForwardStepCompletion(
		ctx,
		q.Tx,
		sagaID,
		step,
		attemptCount,
		completedAt,
	)
}

func (q txQueryer) LockLatestUncompensatedForwardStepCompletion(
	ctx context.Context,
	sagaID model.SagaID,
) (model.SagaForwardStepCompletion, error) {
	return LockLatestUncompensatedForwardStepCompletion(ctx, q.Tx, sagaID)
}

func (q txQueryer) ResolveForwardStepCompensation(
	ctx context.Context,
	completionID model.SagaStepCompletionID,
	compensationState model.SagaStepCompensationState,
	resolvedAt time.Time,
) (model.SagaForwardStepCompletion, error) {
	return ResolveForwardStepCompensation(
		ctx,
		q.Tx,
		completionID,
		compensationState,
		resolvedAt,
	)
}
