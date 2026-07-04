package sagarp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const sagaForwardStepCompletionColumns = `
	id,
	saga_id,
	step,
	sequence,
	attempt_count,
	compensation_state,
	completed_at,
	compensation_resolved_at
`

// ListForwardStepCompletions returns forward-step completion history in durable
// completion order.
func ListForwardStepCompletions[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	sagaID model.SagaID,
) ([]model.SagaForwardStepCompletion, error) {
	if sagaID.IsZero() {
		return nil, cerr.InvalidArgument(errors.New("saga id is required"))
	}

	const query = `SELECT` + sagaForwardStepCompletionColumns + `
FROM saga_forward_step_completions
WHERE saga_id = $1
ORDER BY sequence ASC
`

	return postgres.QueryAll[model.SagaForwardStepCompletion](
		ctx,
		q,
		"list saga forward step completions",
		query,
		sagaID,
	)
}

// RecordForwardStepCompletion appends a successful forward-step completion.
//
// The Saga row is locked before Sequence is allocated. The row lock is held by
// the surrounding transaction, which serializes completion-order allocation for
// one Saga without introducing a global sequence or relying on timestamps.
func RecordForwardStepCompletion[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	sagaID model.SagaID,
	step model.SagaStep,
	attemptCount int,
	completedAt time.Time,
) (model.SagaForwardStepCompletion, error) {
	const op = "sagarp.RecordForwardStepCompletion"

	if sagaID.IsZero() {
		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(errors.New("saga id is required"))
	}

	if !step.IsValid() {
		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(errors.New("saga step is required"))
	}

	if len(step) > maxSagaStepLength {
		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(fmt.Errorf(
			"%s: saga step must not exceed %d characters", op, maxSagaStepLength,
		))
	}

	if attemptCount <= 0 {
		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(
			errors.New("saga step attempt count must be positive"),
		)
	}

	if err := postgres.RequiredTime("completed_at", completedAt); err != nil {
		return model.SagaForwardStepCompletion{}, err
	}

	instance, err := LockByID(ctx, q, sagaID)
	if err != nil {
		return model.SagaForwardStepCompletion{}, err
	}

	if instance.Status.IsTerminal() {
		return model.SagaForwardStepCompletion{}, cerr.Conflict(
			fmt.Errorf("%s: cannot record completion for terminal saga %s", op, sagaID),
		)
	}

	if instance.Phase != model.ForwardSagaPhase {
		return model.SagaForwardStepCompletion{}, cerr.Conflict(fmt.Errorf(
			"%s: cannot record forward completion while saga %s is in phase %q", op, sagaID,
			instance.Phase,
		))
	}

	if instance.CurrentStep != step {
		return model.SagaForwardStepCompletion{}, cerr.Conflict(fmt.Errorf(
			"%s: saga %s current step %q does not match completed step %q", op, sagaID,
			instance.CurrentStep,
			step,
		))
	}

	if instance.StepAttemptCount != attemptCount {
		return model.SagaForwardStepCompletion{}, cerr.Conflict(fmt.Errorf(
			"%s: saga %s current attempt %d does not match completed attempt %d", op, sagaID,
			instance.StepAttemptCount,
			attemptCount,
		))
	}

	const query = `
INSERT INTO saga_forward_step_completions (
	saga_id,
	step,
	sequence,
	attempt_count,
	compensation_state,
	completed_at,
	compensation_resolved_at
)
VALUES (
	$1,
	$2,
	(
		SELECT COALESCE(MAX(sequence), 0) + 1
		FROM saga_forward_step_completions
		WHERE saga_id = $1
	),
	$3,
	$4,
	$5,
	NULL
)
RETURNING` + sagaForwardStepCompletionColumns

	completion, err := postgres.QueryOne[model.SagaForwardStepCompletion](
		ctx,
		q,
		"record saga forward step completion",
		query,
		sagaID,
		step,
		attemptCount,
		model.SagaStepNotCompensated,
		completedAt,
	)
	if err != nil {
		return model.SagaForwardStepCompletion{}, err
	}

	if err = completion.Validate(); err != nil {
		return model.SagaForwardStepCompletion{}, cerr.InternalError(
			fmt.Errorf("%s: recorded saga step completion violates durable model invariants: %w", op, err),
		)
	}

	return completion, nil
}

// LockLatestUncompensatedForwardStepCompletion returns the most recent forward
// completion whose compensation is still unresolved and locks that completion
// row for the current transaction.
func LockLatestUncompensatedForwardStepCompletion[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	sagaID model.SagaID,
) (model.SagaForwardStepCompletion, error) {
	if sagaID.IsZero() {
		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(errors.New("saga id is required"))
	}

	const query = `SELECT` + sagaForwardStepCompletionColumns + `
FROM saga_forward_step_completions
WHERE saga_id = $1
  AND compensation_state = $2
ORDER BY sequence DESC
LIMIT 1
FOR UPDATE
`

	return postgres.QueryOne[model.SagaForwardStepCompletion](
		ctx,
		q,
		"lock latest uncompensated saga forward step completion",
		query,
		sagaID,
		model.SagaStepNotCompensated,
	)
}

// ResolveForwardStepCompensation marks one forward-step completion as
// compensated or explicitly not requiring compensation.
func ResolveForwardStepCompensation[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	completionID model.SagaStepCompletionID,
	compensationState model.SagaStepCompensationState,
	resolvedAt time.Time,
) (model.SagaForwardStepCompletion, error) {
	const op = "sagarp.ResolveForwardStepCompensation"

	if completionID.IsZero() {
		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(
			errors.New("saga step completion id is required"),
		)
	}

	if !compensationState.IsTerminal() {
		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(fmt.Errorf(
			"%s: saga step compensation state %q must be terminal", op, compensationState,
		))
	}

	if err := postgres.RequiredTime("resolved_at", resolvedAt); err != nil {
		return model.SagaForwardStepCompletion{}, err
	}

	current, err := lockForwardStepCompletionByID(ctx, q, completionID)
	if err != nil {
		return model.SagaForwardStepCompletion{}, err
	}

	if current.CompensationState.IsTerminal() {
		return model.SagaForwardStepCompletion{}, cerr.Conflict(fmt.Errorf(
			"%s: %w: completion %s", op, repo.ErrSagaCompletionAlreadyResolved,
			completionID,
		))
	}

	next := current
	next.CompensationState = compensationState
	next.CompensationResolvedAt = &resolvedAt
	if err = current.ValidateTransitionTo(next); err != nil {
		if errors.Is(err, model.ErrInvalidStatusTransition) {
			return model.SagaForwardStepCompletion{}, cerr.Conflict(err)
		}

		return model.SagaForwardStepCompletion{}, cerr.InvalidArgument(err)
	}

	const query = `
UPDATE saga_forward_step_completions
SET
	compensation_state = $2,
	compensation_resolved_at = $3
WHERE id = $1
  AND compensation_state = $4
RETURNING` + sagaForwardStepCompletionColumns

	updated, err := postgres.QueryOne[model.SagaForwardStepCompletion](
		ctx,
		q,
		"resolve saga forward step compensation",
		query,
		completionID,
		compensationState,
		resolvedAt,
		model.SagaStepNotCompensated,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.SagaForwardStepCompletion{}, cerr.Conflict(fmt.Errorf(
				"%s: %w: completion %s", op, repo.ErrSagaCompletionAlreadyResolved,
				completionID,
			))
		}

		return model.SagaForwardStepCompletion{}, err
	}

	if err = updated.Validate(); err != nil {
		return model.SagaForwardStepCompletion{}, cerr.InternalError(
			fmt.Errorf("%s: resolved saga step completion violates durable model invariants: %w", op, err),
		)
	}

	return updated, nil
}

func lockForwardStepCompletionByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	completionID model.SagaStepCompletionID,
) (model.SagaForwardStepCompletion, error) {
	const query = `SELECT` + sagaForwardStepCompletionColumns + `
FROM saga_forward_step_completions
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.SagaForwardStepCompletion](
		ctx,
		q,
		"lock saga forward step completion by id",
		query,
		completionID,
	)
}
