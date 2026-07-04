package sagarp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	maxSagaTypeLength      = 128
	maxBusinessKeyLength   = 512
	maxSagaStepLength      = 128
	maxSubjectTypeLength   = 128
	maxSubjectIDLength     = 256
	maxWorkerIDLength      = 128
	maxCorrelationIDLength = 256
	maxCausationIDLength   = 256
	maxLastErrorLength     = 4096
)

const sagaColumns = `
	id,
	saga_type,
	definition_version,
	business_key,
	subject_type,
	subject_id,
	phase,
	status,
	current_step,
	step_attempt_count,
	next_attempt_at,
	lease_until,
	locked_by,
	version,
	last_error,
	correlation_id,
	causation_id,
	created_at,
	updated_at,
	finished_at
`

// GetByID returns a durable Saga instance by identifier.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.SagaID,
) (model.SagaInstance, error) {
	if id.IsZero() {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("saga id is required"))
	}

	const query = `SELECT` + sagaColumns + `
FROM saga_instances
WHERE id = $1
`

	return postgres.QueryOne[model.SagaInstance](
		ctx,
		q,
		"get saga by id",
		query,
		id,
	)
}

// GetByTypeAndBusinessKey returns the unique logical Saga identified by type
// and business key.
func GetByTypeAndBusinessKey[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	sagaType model.SagaType,
	businessKey model.SagaBusinessKey,
) (model.SagaInstance, error) {
	if err := validateSagaIdentity(sagaType, businessKey); err != nil {
		return model.SagaInstance{}, err
	}

	const query = `SELECT` + sagaColumns + `
FROM saga_instances
WHERE saga_type = $1
  AND business_key = $2
`

	return postgres.QueryOne[model.SagaInstance](
		ctx,
		q,
		"get saga by type and business key",
		query,
		sagaType,
		businessKey,
	)
}

// Create persists the initial durable state of a Saga inside the caller's
// transaction.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
	const op = "sagarp.Create"

	candidate := model.SagaInstance{
		// The database owns the durable identifier. A non-zero placeholder lets
		// the shared model validate all other creation invariants before INSERT.
		ID:                model.SagaID{1},
		Type:              sagaType,
		DefinitionVersion: definitionVersion,
		BusinessKey:       businessKey,
		SubjectType:       subjectType,
		SubjectID:         subjectID,
		Phase:             phase,
		Status:            status,
		CurrentStep:       currentStep,
		StepAttemptCount:  0,
		NextAttemptAt:     nextAttemptAt,
		Version:           0,
		CorrelationID:     correlationID,
		CausationID:       causationID,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}

	if err := validateSagaForPersistence(candidate); err != nil {
		return model.SagaInstance{}, err
	}

	const query = `
INSERT INTO saga_instances (
	saga_type,
	definition_version,
	business_key,
	subject_type,
	subject_id,
	phase,
	status,
	current_step,
	step_attempt_count,
	next_attempt_at,
	lease_until,
	locked_by,
	version,
	last_error,
	correlation_id,
	causation_id,
	created_at,
	updated_at,
	finished_at
)
VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	0, $9, NULL, NULL, 0, NULL, $10, $11, $12, $13, NULL
)
RETURNING` + sagaColumns

	instance, err := postgres.QueryOne[model.SagaInstance](
		ctx,
		q,
		"create saga",
		query,
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
	if err != nil {
		if postgres.IsUniqueViolation(err, "uq_saga_instances_type_business_key") {
			return model.SagaInstance{}, cerr.Conflict(
				fmt.Errorf(
					"%s: %w: type %q and business key %q",
					op,
					repo.ErrSagaAlreadyExists,
					sagaType,
					businessKey,
				),
			)
		}

		return model.SagaInstance{}, err
	}

	if err = instance.Validate(); err != nil {
		return model.SagaInstance{}, cerr.InternalError(
			fmt.Errorf("%s: created saga violates durable model invariants: %w", op, err),
		)
	}

	return instance, nil
}

// LockByID returns and row-locks a Saga instance for the current transaction.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.SagaID,
) (model.SagaInstance, error) {
	if id.IsZero() {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("saga id is required"))
	}

	const query = `SELECT` + sagaColumns + `
FROM saga_instances
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.SagaInstance](
		ctx,
		q,
		"lock saga by id",
		query,
		id,
	)
}

// ClaimByID atomically acquires a runnable Saga for one worker and returns the
// post-claim fencing version.
func ClaimByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.SagaID,
	workerID string,
	now time.Time,
	leaseDuration time.Duration,
) (model.SagaInstance, error) {
	const op = "sagarp.ClaimByID"

	if id.IsZero() {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("saga id is required"))
	}

	if err := validateClaim(workerID, now, leaseDuration); err != nil {
		return model.SagaInstance{}, err
	}

	leaseUntil := now.Add(leaseDuration)

	const query = `
UPDATE saga_instances
SET
	locked_by = $2,
	lease_until = $3,
	step_attempt_count = step_attempt_count + 1,
	version = version + 1,
	updated_at = GREATEST(updated_at, $4)
WHERE id = $1
  AND status = $5
  AND (next_attempt_at IS NULL OR next_attempt_at <= $4)
  AND (lease_until IS NULL OR lease_until <= $4)
RETURNING` + sagaColumns

	instance, err := postgres.QueryOne[model.SagaInstance](
		ctx,
		q,
		"claim saga by id",
		query,
		id,
		workerID,
		leaseUntil,
		now,
		model.RunningSagaStatus,
	)
	if err == nil {
		if validateErr := instance.Validate(); validateErr != nil {
			return model.SagaInstance{}, cerr.InternalError(
				fmt.Errorf("%s: claimed saga violates durable model invariants: %w", op, validateErr),
			)
		}

		return instance, nil
	}

	if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.SagaInstance{}, err
	}

	// UPDATE ... RETURNING cannot distinguish a missing row from an existing
	// row that is not currently runnable. Resolve that distinction only on the
	// unsuccessful path so successful claims remain one database round-trip.
	if _, getErr := GetByID(ctx, q, id); getErr != nil {
		return model.SagaInstance{}, getErr
	}

	return model.SagaInstance{}, cerr.Conflict(
		fmt.Errorf("%s: %w: saga %s", op, repo.ErrSagaNotRunnable, id),
	)
}

// ClaimDue atomically leases a batch of due Saga instances. SKIP LOCKED allows
// multiple workers and replicas to claim independent work without blocking one
// another.
func ClaimDue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	workerID string,
	limit int,
	now time.Time,
	leaseDuration time.Duration,
) ([]model.SagaInstance, error) {
	const op = "sagarp.ClaimDue"

	if limit <= 0 {
		return nil, cerr.InvalidArgument(errors.New("saga claim limit must be positive"))
	}

	if err := validateClaim(workerID, now, leaseDuration); err != nil {
		return nil, err
	}

	leaseUntil := now.Add(leaseDuration)

	const query = `
WITH picked AS (
	SELECT id
	FROM saga_instances
	WHERE status = $1
	  AND (next_attempt_at IS NULL OR next_attempt_at <= $2)
	  AND (lease_until IS NULL OR lease_until <= $2)
	ORDER BY COALESCE(next_attempt_at, created_at) ASC, created_at ASC, id ASC
	LIMIT $3
	FOR UPDATE SKIP LOCKED
)
UPDATE saga_instances AS s
SET
	locked_by = $4,
	lease_until = $5,
	step_attempt_count = s.step_attempt_count + 1,
	version = s.version + 1,
	updated_at = GREATEST(s.updated_at, $2)
FROM picked
WHERE s.id = picked.id
RETURNING
	s.id,
	s.saga_type,
	s.definition_version,
	s.business_key,
	s.subject_type,
	s.subject_id,
	s.phase,
	s.status,
	s.current_step,
	s.step_attempt_count,
	s.next_attempt_at,
	s.lease_until,
	s.locked_by,
	s.version,
	s.last_error,
	s.correlation_id,
	s.causation_id,
	s.created_at,
	s.updated_at,
	s.finished_at
`

	instances, err := postgres.QueryAll[model.SagaInstance](
		ctx,
		q,
		"claim due sagas",
		query,
		model.RunningSagaStatus,
		now,
		limit,
		workerID,
		leaseUntil,
	)
	if err != nil {
		return nil, err
	}

	for i := range instances {
		if validateErr := instances[i].Validate(); validateErr != nil {
			return nil, cerr.InternalError(
				fmt.Errorf("%s: claimed saga violates durable model invariants: %w", op, validateErr),
			)
		}
	}

	return instances, nil
}

// Update persists one generic durable Saga state transition using expectedVersion
// as the fencing/CAS condition. The active worker lease is cleared on success.
func Update[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
	const op = "sagarp.Update"

	if id.IsZero() {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("saga id is required"))
	}

	if expectedVersion < 0 {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("expected saga version must not be negative"))
	}

	if stepAttemptCount < 0 {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("step attempt count must not be negative"))
	}

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.SagaInstance{}, err
	}

	if nextAttemptAt != nil && nextAttemptAt.IsZero() {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("next_attempt_at must not be zero"))
	}

	if finishedAt != nil && finishedAt.IsZero() {
		return model.SagaInstance{}, cerr.InvalidArgument(errors.New("finished_at must not be zero"))
	}

	if lastError != nil && len(*lastError) > maxLastErrorLength {
		return model.SagaInstance{}, cerr.InvalidArgument(fmt.Errorf(
			"%s: last_error must not exceed %d characters", op, maxLastErrorLength,
		))
	}

	current, err := LockByID(ctx, q, id)
	if err != nil {
		return model.SagaInstance{}, err
	}

	if current.Version != expectedVersion {
		return model.SagaInstance{}, cerr.Conflict(fmt.Errorf(
			"%s: %w: saga %s expected version %d, current version %d",
			op,
			repo.ErrSagaVersionConflict,
			id,
			expectedVersion,
			current.Version,
		))
	}

	next := current
	next.Phase = phase
	next.Status = status
	next.CurrentStep = currentStep
	next.StepAttemptCount = stepAttemptCount
	next.NextAttemptAt = nextAttemptAt
	next.LeaseUntil = nil
	next.LockedBy = nil
	next.Version = current.Version + 1
	next.LastError = lastError
	next.FinishedAt = finishedAt
	next.UpdatedAt = updatedAt

	if err = validateSagaForPersistence(next); err != nil {
		return model.SagaInstance{}, err
	}

	if err = current.ValidateTransitionTo(next); err != nil {
		if errors.Is(err, model.ErrInvalidStatusTransition) {
			return model.SagaInstance{}, cerr.Conflict(err)
		}

		return model.SagaInstance{}, cerr.InvalidArgument(err)
	}

	const query = `
UPDATE saga_instances
SET
	phase = $3,
	status = $4,
	current_step = $5,
	step_attempt_count = $6,
	next_attempt_at = $7,
	lease_until = NULL,
	locked_by = NULL,
	version = version + 1,
	last_error = $8,
	finished_at = $9,
	updated_at = $10
WHERE id = $1
  AND version = $2
RETURNING` + sagaColumns

	updated, err := postgres.QueryOne[model.SagaInstance](
		ctx,
		q,
		"update saga",
		query,
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
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.SagaInstance{}, cerr.Conflict(fmt.Errorf(
				"%s: %w: saga %s expected version %d",
				op,
				repo.ErrSagaVersionConflict,
				id,
				expectedVersion,
			))
		}

		return model.SagaInstance{}, err
	}

	if err = updated.Validate(); err != nil {
		return model.SagaInstance{}, cerr.InternalError(
			fmt.Errorf("%s: updated saga violates durable model invariants: %w", op, err),
		)
	}

	return updated, nil
}

func validateClaim(workerID string, now time.Time, leaseDuration time.Duration) error {
	const op = "sagarp.validateClaim"

	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return cerr.InvalidArgument(errors.New("saga worker id is required"))
	}

	if len(workerID) > maxWorkerIDLength {
		return cerr.InvalidArgument(fmt.Errorf(
			"%s: saga worker id must not exceed %d characters", op, maxWorkerIDLength,
		))
	}

	if err := postgres.RequiredTime("now", now); err != nil {
		return err
	}

	if leaseDuration <= 0 {
		return cerr.InvalidArgument(errors.New("saga lease duration must be positive"))
	}

	return nil
}

func validateSagaIdentity(
	sagaType model.SagaType,
	businessKey model.SagaBusinessKey,
) error {
	const op = "sagarp.validateSagaIdentity"

	if !sagaType.IsValid() {
		return cerr.InvalidArgument(errors.New("saga type is required"))
	}

	if len(sagaType) > maxSagaTypeLength {
		return cerr.InvalidArgument(fmt.Errorf(
			"%s: saga type must not exceed %d characters", op, maxSagaTypeLength,
		))
	}

	if !businessKey.IsValid() {
		return cerr.InvalidArgument(errors.New("saga business key is required"))
	}

	if len(businessKey) > maxBusinessKeyLength {
		return cerr.InvalidArgument(fmt.Errorf(
			"%s: saga business key must not exceed %d characters", op, maxBusinessKeyLength,
		))
	}

	return nil
}

func validateSagaForPersistence(instance model.SagaInstance) error {
	const op = "sagarp.validateSagaForPersistence"

	if err := validateSagaIdentity(instance.Type, instance.BusinessKey); err != nil {
		return err
	}

	if len(instance.CurrentStep) > maxSagaStepLength {
		return cerr.InvalidArgument(fmt.Errorf(
			"%s: saga step must not exceed %d characters", op, maxSagaStepLength,
		))
	}

	if err := validateOptionalText("subject_type", instance.SubjectType, maxSubjectTypeLength); err != nil {
		return err
	}

	if err := validateOptionalText("subject_id", instance.SubjectID, maxSubjectIDLength); err != nil {
		return err
	}

	if err := validateOptionalText("locked_by", instance.LockedBy, maxWorkerIDLength); err != nil {
		return err
	}

	if err := validateOptionalText("correlation_id", instance.CorrelationID, maxCorrelationIDLength); err != nil {
		return err
	}

	if err := validateOptionalText("causation_id", instance.CausationID, maxCausationIDLength); err != nil {
		return err
	}

	if instance.LastError != nil && len(*instance.LastError) > maxLastErrorLength {
		return cerr.InvalidArgument(fmt.Errorf(
			"%s: last_error must not exceed %d characters", op, maxLastErrorLength,
		))
	}

	if instance.NextAttemptAt != nil && instance.NextAttemptAt.IsZero() {
		return cerr.InvalidArgument(errors.New("next_attempt_at must not be zero"))
	}

	if instance.FinishedAt != nil && instance.FinishedAt.IsZero() {
		return cerr.InvalidArgument(errors.New("finished_at must not be zero"))
	}

	if err := instance.Validate(); err != nil {
		return cerr.InvalidArgument(err)
	}

	return nil
}

func validateOptionalText(field string, value *string, maxLen int) error {
	const op = "sagarp.validateOptionalText"

	if value == nil {
		return nil
	}

	if strings.TrimSpace(*value) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: %s must not be blank", op, field))
	}

	if len(*value) > maxLen {
		return cerr.InvalidArgument(fmt.Errorf(
			"%s: %s must not exceed %d characters", op, field,
			maxLen,
		))
	}

	return nil
}
