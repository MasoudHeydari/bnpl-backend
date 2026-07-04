package repo

import (
	"context"
	"errors"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var (
	// ErrSagaAlreadyExists indicates that a Saga business identity is already
	// bound to a durable instance. Callers may load the existing instance to
	// determine whether the request is an idempotent replay.
	ErrSagaAlreadyExists = errors.New("saga already exists")

	// ErrSagaNotRunnable indicates that a Saga cannot currently be claimed
	// because it is not running, is not due, or has an active worker lease.
	ErrSagaNotRunnable = errors.New("saga is not runnable")

	// ErrSagaVersionConflict indicates that a Saga update used a stale fencing
	// version and must not overwrite newer durable state.
	ErrSagaVersionConflict = errors.New("saga version conflict")

	// ErrSagaCompletionAlreadyResolved indicates that compensation resolution was
	// attempted for a forward-step completion that is already terminal.
	ErrSagaCompletionAlreadyResolved = errors.New("saga step completion already resolved")
)

// SagaQueryer defines read-only operations over durable Saga process state.
type SagaQueryer interface {
	// GetByID returns a Saga instance by its durable identifier.
	GetByID(
		ctx context.Context,
		id model.SagaID,
	) (model.SagaInstance, error)

	// GetByTypeAndBusinessKey returns the unique logical Saga instance associated
	// with the supplied Saga type and business identity.
	//
	// The pair (sagaType, businessKey) represents the idempotent business identity
	// of a Saga and should normally be protected by a unique persistence
	// constraint.
	GetByTypeAndBusinessKey(
		ctx context.Context,
		sagaType model.SagaType,
		businessKey model.SagaBusinessKey,
	) (model.SagaInstance, error)

	// ListForwardStepCompletions returns the durable forward-step completion
	// history in ascending Sequence order.
	//
	// The history is append-only except for compensation-resolution metadata and
	// must not be reconstructed from the current Saga snapshot.
	ListForwardStepCompletions(
		ctx context.Context,
		sagaID model.SagaID,
	) ([]model.SagaForwardStepCompletion, error)
}

// SagaConnQueryer exposes connection-scoped operations used primarily by
// workers that claim runnable Saga instances.
type SagaConnQueryer interface {
	// SagaQueryer provides shared Saga query operations.
	SagaQueryer

	// ClaimByID atomically claims one runnable Saga instance by identifier.
	//
	// This operation provides the same leasing and fencing guarantees as
	// ClaimDue and is intended for low-latency execution paths that already know
	// which Saga should run.
	//
	// A successful claim must atomically:
	//
	//   - verify the Saga is currently runnable and has no active lease
	//   - assign LockedBy to workerID
	//   - set LeaseUntil based on leaseDuration
	//   - increment StepAttemptCount for the current step
	//   - increment Version, producing a new fencing token
	//
	// The returned instance must contain the post-claim values.
	ClaimByID(
		ctx context.Context,
		id model.SagaID,
		workerID string,
		now time.Time,
		leaseDuration time.Duration,
	) (model.SagaInstance, error)

	// ClaimDue atomically claims runnable Saga instances whose execution is due
	// and which do not currently have an active worker lease.
	//
	// A Saga is runnable when its durable execution state allows execution and
	// NextAttemptAt, when present, is not later than now. The Saga phase may be
	// either forward or compensating; ClaimDue must not embed workflow-specific
	// knowledge about individual Saga types or steps.
	//
	// Each successful claim must atomically:
	//
	//   - assign LockedBy to workerID
	//   - set LeaseUntil based on leaseDuration
	//   - increment StepAttemptCount for the current step
	//   - increment Version, producing a new fencing token
	//
	// Concurrent workers must not receive the same active claim.
	//
	// Version is important beyond ordinary optimistic locking. If a worker loses
	// its lease and another worker subsequently claims the Saga, the second claim
	// increments Version. The stale worker can therefore no longer persist its
	// result using the older version.
	ClaimDue(
		ctx context.Context,
		workerID string,
		limit int,
		now time.Time,
		leaseDuration time.Duration,
	) ([]model.SagaInstance, error)
}

// SagaTxQueryer exposes transaction-scoped operations over durable Saga state.
//
// Related local state changes, Saga state transitions, and step-completion
// history should use the same transaction whenever they form one local atomic
// unit of work.
type SagaTxQueryer interface {
	// SagaQueryer provides shared Saga query operations.
	SagaQueryer

	// Create stores a new Saga instance inside the current transaction.
	//
	// The component starting the Saga should normally create its durable state
	// atomically with the local state change that starts the distributed process.
	// definitionVersion must identify the immutable workflow definition used to
	// interpret the new instance. The repository persists the supplied state; it
	// must not invent steps, transitions, retry policy, or compensation behavior.
	Create(
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
	) (model.SagaInstance, error)

	// LockByID returns a Saga instance while acquiring a database row lock for
	// the current transaction.
	//
	// The lock serializes local transactional transitions. Version remains the
	// durable fencing mechanism across separate transactions and worker leases.
	LockByID(
		ctx context.Context,
		id model.SagaID,
	) (model.SagaInstance, error)

	// Update persists an already validated Saga state transition.
	//
	// expectedVersion is used as a compare-and-swap condition. If the stored
	// Version differs from expectedVersion, the repository must report a
	// conflict and must not overwrite the newer Saga state.
	//
	// On success, Update must:
	//
	//   - persist the supplied execution state
	//   - increment Version
	//   - clear any active worker lease
	//
	// StepAttemptCount belongs to the current step operation. When CurrentStep or
	// Phase changes, the caller must reset StepAttemptCount to zero before the
	// new operation is claimed.
	//
	// A nil NextAttemptAt means that no time-based retry is scheduled. A Saga
	// waiting for an external trigger should normally have WaitingSagaStatus and
	// a nil NextAttemptAt.
	//
	// The repository must not decide whether a failure means retry,
	// compensation, reconciliation, manual review, or termination.
	Update(
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
	) (model.SagaInstance, error)

	// RecordForwardStepCompletion appends a durable record that the current
	// forward step completed successfully.
	//
	// The repository must allocate Sequence monotonically within sagaID. The
	// completion should be recorded in the same local transaction as the related
	// Saga transition and local state changes whenever atomicity is required.
	RecordForwardStepCompletion(
		ctx context.Context,
		sagaID model.SagaID,
		step model.SagaStep,
		attemptCount int,
		completedAt time.Time,
	) (model.SagaForwardStepCompletion, error)

	// LockLatestUncompensatedForwardStepCompletion returns and row-locks the most
	// recently completed forward step whose compensation has not yet been
	// resolved.
	//
	// Compensation uses reverse completion order. Callers must not infer this
	// order from SagaInstance.CurrentStep because the current step is only a
	// snapshot of the process position.
	LockLatestUncompensatedForwardStepCompletion(
		ctx context.Context,
		sagaID model.SagaID,
	) (model.SagaForwardStepCompletion, error)

	// ResolveForwardStepCompensation marks a completed forward step as either
	// compensated or explicitly not requiring compensation.
	//
	// compensationState must be a terminal SagaStepCompensationState. This update
	// should be committed atomically with the Saga transition and any local state
	// change that completes the corresponding compensation action.
	ResolveForwardStepCompensation(
		ctx context.Context,
		completionID model.SagaStepCompletionID,
		compensationState model.SagaStepCompensationState,
		resolvedAt time.Time,
	) (model.SagaForwardStepCompletion, error)
}

// Saga defines repository bindings for durable Saga process-manager state.
//
// The interface follows the project's existing repository boundary:
//
//   - Conn is used for connection-scoped reads and worker claiming.
//   - Tx is used whenever Saga state must participate in a local database
//     transaction together with related local state.
type Saga interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) SagaConnQueryer

	// Tx binds the repository to the current transaction scope.
	Tx(Tx) SagaTxQueryer
}
