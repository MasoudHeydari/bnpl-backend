package model

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SagaID uniquely identifies a SagaInstance.
type SagaID uuid.UUID

// ParseSagaID parses a SagaID from its canonical string form.
func ParseSagaID(value string) (SagaID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return SagaID{}, fmt.Errorf("parse saga id: %w", err)
	}

	return SagaID(id), nil
}

func (id SagaID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id SagaID) String() string { return uuid.UUID(id).String() }

func (id SagaID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id SagaID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *SagaID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = SagaID(u)

	return nil
}

func (id *SagaID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = SagaID(u)

	return nil
}

func (id SagaID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// SagaStepCompletionID uniquely identifies a SagaForwardStepCompletion row.
type SagaStepCompletionID uuid.UUID

// ParseSagaStepCompletionID parses a SagaStepCompletionID from its canonical string form.
func ParseSagaStepCompletionID(value string) (SagaStepCompletionID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return SagaStepCompletionID{}, fmt.Errorf("parse saga step completion id: %w", err)
	}

	return SagaStepCompletionID(id), nil
}

func (id SagaStepCompletionID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id SagaStepCompletionID) String() string { return uuid.UUID(id).String() }

func (id SagaStepCompletionID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id SagaStepCompletionID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *SagaStepCompletionID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = SagaStepCompletionID(u)

	return nil
}

func (id *SagaStepCompletionID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = SagaStepCompletionID(u)

	return nil
}

func (id SagaStepCompletionID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

var (
	// ErrInvalidSagaInstance is returned when durable Saga metadata violates a
	// generic Saga invariant. Business-specific workflow invariants remain the
	// responsibility of the use case that owns the Saga type.
	ErrInvalidSagaInstance = errors.New("invalid saga instance")

	// ErrInvalidSagaStepCompletion is returned when a durable forward-step
	// completion record violates a generic Saga invariant.
	ErrInvalidSagaStepCompletion = errors.New("invalid saga step completion")
)

// SagaType identifies a business process managed as a durable Saga.
//
// Saga types are intentionally open-ended. Concrete values belong to the
// business use case that owns each process rather than this generic model.
type SagaType string

// IsValid reports whether the Saga type has a usable stable identifier.
func (t SagaType) IsValid() bool {
	return strings.TrimSpace(string(t)) != ""
}

// SagaDefinitionVersion identifies the immutable workflow-definition version
// used to interpret a durable Saga instance.
//
// Long-running Saga instances may survive application deployments. Persisting
// the definition version prevents an in-flight instance from being interpreted
// by a different step graph or compensation policy after a rollout.
type SagaDefinitionVersion uint32

// IsValid reports whether the definition version is usable.
func (v SagaDefinitionVersion) IsValid() bool {
	return v > 0
}

// SagaBusinessKey is the stable business identity of a Saga instance within a
// Saga type. Reusing the same type and business key must address the same
// logical process so Saga creation can be idempotent.
type SagaBusinessKey string

// IsValid reports whether the Saga business key is usable.
func (k SagaBusinessKey) IsValid() bool {
	return strings.TrimSpace(string(k)) != ""
}

// SagaStep identifies the current durable business step of a Saga.
//
// Step names are owned by the corresponding business use case. Compensation
// actions should be represented by explicit step names just like forward
// actions; the generic Saga layer does not call a magic rollback operation.
type SagaStep string

// IsValid reports whether the Saga step has a usable stable identifier.
func (s SagaStep) IsValid() bool {
	return strings.TrimSpace(string(s)) != ""
}

// SagaPhase identifies whether the process is moving forward or executing
// compensating business actions.
//
// Phase is deliberately separate from SagaStatus. For example, a compensation
// step may itself be retried while the Saga remains in the compensating phase.
type SagaPhase string

const (
	// ForwardSagaPhase means the Saga is executing its normal forward path.
	ForwardSagaPhase SagaPhase = "forward"

	// CompensatingSagaPhase means the Saga is executing explicit compensating
	// business actions for previously committed local transactions.
	CompensatingSagaPhase SagaPhase = "compensating"
)

var sagaPhaseTransitions = newStatusTransitionTable(
	"saga phase",
	map[SagaPhase][]SagaPhase{
		ForwardSagaPhase: {
			CompensatingSagaPhase,
		},
		CompensatingSagaPhase: {},
	},
	nil,
)

// IsValid reports whether the Saga phase is known by the generic Saga model.
func (p SagaPhase) IsValid() bool {
	return sagaPhaseTransitions.isValid(p)
}

// CanTransitionTo reports whether the Saga phase may change to next.
func (p SagaPhase) CanTransitionTo(next SagaPhase) bool {
	return sagaPhaseTransitions.canTransitionTo(p, next)
}

// ValidateTransitionTo returns an error when the Saga phase may not change to next.
func (p SagaPhase) ValidateTransitionTo(next SagaPhase) error {
	return sagaPhaseTransitions.validateTransitionTo(p, next)
}

// SagaStatus identifies the durable execution state of a Saga instance.
//
// Retry is not represented as a separate status. A retryable Saga remains
// running and uses NextAttemptAt to delay the next claim. Keeping retry timing
// orthogonal to status also works for compensation retries without losing the
// Saga's phase.
type SagaStatus string

const (
	// RunningSagaStatus means the current step is eligible for execution once
	// NextAttemptAt is due and no active worker lease exists.
	RunningSagaStatus SagaStatus = "running"

	// WaitingSagaStatus means the Saga is waiting for an external fact or
	// business trigger and must not be polled as due work.
	WaitingSagaStatus SagaStatus = "waiting"

	// ReconciliationRequiredSagaStatus means automatic execution is
	// paused because an authoritative external outcome is ambiguous or local
	// state disagrees with an authority boundary.
	ReconciliationRequiredSagaStatus SagaStatus = "reconciliation_required"

	// ManualReviewRequiredSagaStatus means automatic execution is paused until a
	// governed human or operational process resolves the condition.
	ManualReviewRequiredSagaStatus SagaStatus = "manual_review_required"

	// CompletedSagaStatus means the normal forward business process finished.
	CompletedSagaStatus SagaStatus = "completed"

	// CompensatedSagaStatus means the Saga finished its explicit compensation
	// path after previously committed forward work had to be undone semantically.
	CompensatedSagaStatus SagaStatus = "compensated"

	// FailedSagaStatus means the Saga reached a terminal failure that the owning
	// business process has explicitly decided not to continue automatically.
	FailedSagaStatus SagaStatus = "failed"
)

var sagaStatusTransitions = newStatusTransitionTable(
	"saga status",
	map[SagaStatus][]SagaStatus{
		RunningSagaStatus: {
			WaitingSagaStatus,
			ReconciliationRequiredSagaStatus,
			ManualReviewRequiredSagaStatus,
			CompletedSagaStatus,
			CompensatedSagaStatus,
			FailedSagaStatus,
		},
		WaitingSagaStatus: {
			RunningSagaStatus,
			ReconciliationRequiredSagaStatus,
			ManualReviewRequiredSagaStatus,
			CompletedSagaStatus,
			CompensatedSagaStatus,
			FailedSagaStatus,
		},
		ReconciliationRequiredSagaStatus: {
			RunningSagaStatus,
			ManualReviewRequiredSagaStatus,
			CompletedSagaStatus,
			CompensatedSagaStatus,
			FailedSagaStatus,
		},
		ManualReviewRequiredSagaStatus: {
			RunningSagaStatus,
			CompletedSagaStatus,
			CompensatedSagaStatus,
			FailedSagaStatus,
		},
	},
	[]SagaStatus{
		CompletedSagaStatus,
		CompensatedSagaStatus,
		FailedSagaStatus,
	},
)

// IsValid reports whether the Saga status is known by the generic Saga model.
func (s SagaStatus) IsValid() bool {
	return sagaStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the Saga status may change to next.
func (s SagaStatus) CanTransitionTo(next SagaStatus) bool {
	return sagaStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the Saga status may not change to next.
func (s SagaStatus) ValidateTransitionTo(next SagaStatus) error {
	return sagaStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether no further automatic Saga transition is allowed.
func (s SagaStatus) IsTerminal() bool {
	return sagaStatusTransitions.isTerminal(s)
}

// SagaInstance stores durable process-manager state for one logical Saga.
//
// This model intentionally stores coordination metadata only. Business state
// belongs to the authoritative aggregates/repositories owned by the business
// use case; copying aggregate payloads into Saga state would create a second
// source of truth.
type SagaInstance struct {
	// ID uniquely identifies the durable Saga row.
	ID SagaID `db:"id" json:"id"`

	// Type identifies which business process owns this Saga.
	Type SagaType `db:"saga_type" json:"saga_type"`

	// DefinitionVersion identifies the immutable workflow definition used to
	// interpret this instance.
	DefinitionVersion SagaDefinitionVersion `db:"definition_version" json:"definition_version"`

	// BusinessKey is the stable idempotency/business identity within Type.
	BusinessKey SagaBusinessKey `db:"business_key" json:"business_key"`

	// SubjectType optionally identifies the primary local business subject type.
	// It is a string because not every integration/business subject is guaranteed
	// to use an internal UUID.
	SubjectType *string `db:"subject_type" json:"subject_type,omitempty"`

	// SubjectID optionally identifies the primary local business subject identity.
	SubjectID *string `db:"subject_id" json:"subject_id,omitempty"`

	// Phase distinguishes forward progress from explicit compensation.
	Phase SagaPhase `db:"phase" json:"phase"`

	// Status is the durable execution state.
	Status SagaStatus `db:"status" json:"status"`

	// CurrentStep is the durable step that should execute next or is currently
	// waiting for an external trigger.
	CurrentStep SagaStep `db:"current_step" json:"current_step"`

	// StepAttemptCount counts claims/execution attempts for CurrentStep. It is
	// reset by the owning use case when advancing to a different step.
	StepAttemptCount int `db:"step_attempt_count" json:"step_attempt_count"`

	// NextAttemptAt delays execution of a running Saga. Nil means immediately
	// eligible, subject to worker leasing.
	NextAttemptAt *time.Time `db:"next_attempt_at" json:"next_attempt_at,omitempty"`

	// LeaseUntil is the expiry of the short-lived worker lease used while a
	// handler performs work outside a database transaction.
	LeaseUntil *time.Time `db:"lease_until" json:"lease_until,omitempty"`

	// LockedBy identifies the worker currently holding the Saga lease.
	LockedBy *string `db:"locked_by" json:"locked_by,omitempty"`

	// Version is a fencing/CAS token incremented on every claim and durable
	// state mutation. A stale worker must not overwrite a newer Saga state.
	Version int64 `db:"version" json:"version"`

	// LastError stores bounded internal diagnostic context for the most recent
	// failed execution attempt. It is not a public API field.
	LastError *string `db:"last_error" json:"-"`

	// CorrelationID links the Saga to distributed tracing/business correlation.
	CorrelationID *string `db:"correlation_id" json:"correlation_id,omitempty"`

	// CausationID optionally records the command/event that started the Saga.
	CausationID *string `db:"causation_id" json:"causation_id,omitempty"`

	// CreatedAt is when the Saga instance was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the Saga instance was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	// FinishedAt is set for terminal completed/failed Sagas.
	FinishedAt *time.Time `db:"finished_at" json:"finished_at,omitempty"`
}

// IsDue reports whether the Saga is eligible to be claimed for execution at now.
//
// Repository implementations still need an atomic claim because IsDue alone is
// not a concurrency-control mechanism.
func (s SagaInstance) IsDue(now time.Time) bool {
	if s.Status != RunningSagaStatus {
		return false
	}

	if s.NextAttemptAt != nil && s.NextAttemptAt.After(now) {
		return false
	}

	if s.LeaseUntil != nil && s.LeaseUntil.After(now) {
		return false
	}

	return true
}

// Validate checks generic durable Saga invariants.
//
// It intentionally does not validate concrete Saga types, step names, or the
// business meaning of transitions. Those invariants belong to the owning use
// case and must not leak into this shared model.
func (s SagaInstance) Validate() error {
	const op = "model.SagaInstance.Validate"

	if s.ID.IsZero() {
		return fmt.Errorf("%s: %w: id is required", op, ErrInvalidSagaInstance)
	}

	if !s.Type.IsValid() {
		return fmt.Errorf("%s: %w: saga type is required", op, ErrInvalidSagaInstance)
	}

	if !s.DefinitionVersion.IsValid() {
		return fmt.Errorf("%s: %w: definition version is required", op, ErrInvalidSagaInstance)
	}

	if !s.BusinessKey.IsValid() {
		return fmt.Errorf("%s: %w: business key is required", op, ErrInvalidSagaInstance)
	}

	if !s.Phase.IsValid() {
		return fmt.Errorf("%s: %w: invalid phase %q", op, ErrInvalidSagaInstance, s.Phase)
	}

	if !s.Status.IsValid() {
		return fmt.Errorf("%s: %w: invalid status %q", op, ErrInvalidSagaInstance, s.Status)
	}

	if !s.CurrentStep.IsValid() {
		return fmt.Errorf("%s: %w: current step is required", op, ErrInvalidSagaInstance)
	}

	if s.StepAttemptCount < 0 {
		return fmt.Errorf("%s: %w: step attempt count must not be negative", op, ErrInvalidSagaInstance)
	}

	if s.Version < 0 {
		return fmt.Errorf("%s: %w: version must not be negative", op, ErrInvalidSagaInstance)
	}

	if (s.SubjectType == nil) != (s.SubjectID == nil) {
		return fmt.Errorf("%s: %w: subject type and subject id must be set together", op, ErrInvalidSagaInstance)
	}

	if s.SubjectType != nil && strings.TrimSpace(*s.SubjectType) == "" {
		return fmt.Errorf("%s: %w: subject type must not be blank", op, ErrInvalidSagaInstance)
	}

	if s.SubjectID != nil && strings.TrimSpace(*s.SubjectID) == "" {
		return fmt.Errorf("%s: %w: subject id must not be blank", op, ErrInvalidSagaInstance)
	}

	if (s.LeaseUntil == nil) != (s.LockedBy == nil) {
		return fmt.Errorf("%s: %w: lease and worker identity must be set together", op, ErrInvalidSagaInstance)
	}

	if s.LockedBy != nil && strings.TrimSpace(*s.LockedBy) == "" {
		return fmt.Errorf("%s: %w: locked by must not be blank", op, ErrInvalidSagaInstance)
	}

	if s.LeaseUntil != nil && s.LeaseUntil.IsZero() {
		return fmt.Errorf("%s: %w: lease_until must not be zero", op, ErrInvalidSagaInstance)
	}

	if s.CorrelationID != nil && strings.TrimSpace(*s.CorrelationID) == "" {
		return fmt.Errorf("%s: %w: correlation id must not be blank", op, ErrInvalidSagaInstance)
	}

	if s.CausationID != nil && strings.TrimSpace(*s.CausationID) == "" {
		return fmt.Errorf("%s: %w: causation id must not be blank", op, ErrInvalidSagaInstance)
	}

	if s.Status != RunningSagaStatus && s.NextAttemptAt != nil {
		return fmt.Errorf("%s: %w: only running sagas may schedule a next attempt", op, ErrInvalidSagaInstance)
	}

	if s.Status != RunningSagaStatus && s.LeaseUntil != nil {
		return fmt.Errorf("%s: %w: only running sagas may hold a worker lease", op, ErrInvalidSagaInstance)
	}

	if s.Status == CompletedSagaStatus && s.Phase != ForwardSagaPhase {
		return fmt.Errorf("%s: %w: completed saga must be in forward phase", op, ErrInvalidSagaInstance)
	}

	if s.Status == CompensatedSagaStatus && s.Phase != CompensatingSagaPhase {
		return fmt.Errorf("%s: %w: compensated saga must be in compensating phase", op, ErrInvalidSagaInstance)
	}

	if s.Status.IsTerminal() {
		if s.FinishedAt == nil {
			return fmt.Errorf("%s: %w: terminal saga requires finished_at", op, ErrInvalidSagaInstance)
		}
	} else if s.FinishedAt != nil {
		return fmt.Errorf("%s: %w: non-terminal saga must not have finished_at", op, ErrInvalidSagaInstance)
	}

	if s.CreatedAt.IsZero() {
		return fmt.Errorf("%s: %w: created_at is required", op, ErrInvalidSagaInstance)
	}

	if s.UpdatedAt.IsZero() {
		return fmt.Errorf("%s: %w: updated_at is required", op, ErrInvalidSagaInstance)
	}

	if s.UpdatedAt.Before(s.CreatedAt) {
		return fmt.Errorf("%s: %w: updated_at must not precede created_at", op, ErrInvalidSagaInstance)
	}

	if s.FinishedAt != nil && s.FinishedAt.Before(s.CreatedAt) {
		return fmt.Errorf("%s: %w: finished_at must not precede created_at", op, ErrInvalidSagaInstance)
	}

	return nil
}

// ValidateTransitionTo checks generic status/phase transition rules.
//
// Business-specific step ordering and compensation policy remain the owning
// use case's responsibility.
func (s SagaInstance) ValidateTransitionTo(next SagaInstance) error {
	const op = "model.SagaInstance.ValidateTransitionTo"

	if err := s.Validate(); err != nil {
		return err
	}

	if s.Status.IsTerminal() {
		return fmt.Errorf(
			"%s: %w: terminal saga status %q cannot transition", op, ErrInvalidStatusTransition,
			s.Status,
		)
	}

	if s.ID != next.ID {
		return fmt.Errorf("%s: %w: saga id is immutable", op, ErrInvalidSagaInstance)
	}

	if s.Type != next.Type {
		return fmt.Errorf("%s: %w: saga type is immutable", op, ErrInvalidSagaInstance)
	}

	if s.DefinitionVersion != next.DefinitionVersion {
		return fmt.Errorf("%s: %w: saga definition version is immutable", op, ErrInvalidSagaInstance)
	}

	if s.BusinessKey != next.BusinessKey {
		return fmt.Errorf("%s: %w: saga business key is immutable", op, ErrInvalidSagaInstance)
	}

	if !sameOptionalString(s.SubjectType, next.SubjectType) ||
		!sameOptionalString(s.SubjectID, next.SubjectID) {
		return fmt.Errorf("%s: %w: saga subject identity is immutable", op, ErrInvalidSagaInstance)
	}

	if !sameOptionalString(s.CorrelationID, next.CorrelationID) {
		return fmt.Errorf("%s: %w: saga correlation id is immutable", op, ErrInvalidSagaInstance)
	}

	if !sameOptionalString(s.CausationID, next.CausationID) {
		return fmt.Errorf("%s: %w: saga causation id is immutable", op, ErrInvalidSagaInstance)
	}

	if !s.CreatedAt.Equal(next.CreatedAt) {
		return fmt.Errorf("%s: %w: saga created_at is immutable", op, ErrInvalidSagaInstance)
	}

	if next.UpdatedAt.Before(s.UpdatedAt) {
		return fmt.Errorf("%s: %w: updated_at must not move backwards", op, ErrInvalidSagaInstance)
	}

	if next.Version < s.Version {
		return fmt.Errorf("%s: %w: version must not decrease", op, ErrInvalidSagaInstance)
	}

	operationChanged := s.CurrentStep != next.CurrentStep || s.Phase != next.Phase
	if operationChanged && next.StepAttemptCount != 0 {
		return fmt.Errorf(
			"%s: %w: step attempt count must reset when step or phase changes", op, ErrInvalidSagaInstance,
		)
	}

	if !operationChanged && next.StepAttemptCount < s.StepAttemptCount {
		return fmt.Errorf(
			"%s: %w: step attempt count must not decrease for the current step", op, ErrInvalidSagaInstance,
		)
	}

	if err := s.Status.ValidateTransitionTo(next.Status); err != nil {
		return err
	}

	if err := s.Phase.ValidateTransitionTo(next.Phase); err != nil {
		return err
	}

	if err := next.Validate(); err != nil {
		return err
	}

	return nil
}

// SagaStepCompensationState records whether the committed effect represented by
// a completed forward step has been resolved during compensation.
type SagaStepCompensationState string

const (
	// SagaStepNotCompensated means no compensation resolution has been recorded
	// for the completed forward step.
	SagaStepNotCompensated SagaStepCompensationState = "not_compensated"

	// SagaStepCompensated means the compensating operation or workflow for the
	// completed forward step finished successfully.
	SagaStepCompensated SagaStepCompensationState = "compensated"

	// SagaStepCompensationNotRequired means the Saga definition explicitly
	// declared that the completed forward step requires no compensating action.
	SagaStepCompensationNotRequired SagaStepCompensationState = "not_required"
)

var sagaStepCompensationTransitions = newStatusTransitionTable(
	"saga step compensation state",
	map[SagaStepCompensationState][]SagaStepCompensationState{
		SagaStepNotCompensated: {
			SagaStepCompensated,
			SagaStepCompensationNotRequired,
		},
	},
	[]SagaStepCompensationState{
		SagaStepCompensated,
		SagaStepCompensationNotRequired,
	},
)

// IsValid reports whether the compensation state is recognized by the generic
// Saga model.
func (s SagaStepCompensationState) IsValid() bool {
	return sagaStepCompensationTransitions.isValid(s)
}

// ValidateTransitionTo returns an error when the compensation state may not
// change to next.
func (s SagaStepCompensationState) ValidateTransitionTo(next SagaStepCompensationState) error {
	return sagaStepCompensationTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the compensation state has been permanently
// resolved for the recorded forward step.
func (s SagaStepCompensationState) IsTerminal() bool {
	return sagaStepCompensationTransitions.isTerminal(s)
}

// SagaForwardStepCompletion records that a forward Saga step completed
// successfully and its committed effect may therefore participate in later
// compensation. Completion identity and ordering are immutable; only the
// compensation-resolution metadata may change.
//
// Sequence defines the durable completion order within a Saga. Compensation
// walks these records in reverse sequence order instead of inferring history
// from SagaInstance.CurrentStep, which is only a snapshot of the current
// process state.
type SagaForwardStepCompletion struct {
	// ID uniquely identifies this forward-step completion record.
	ID SagaStepCompletionID `db:"id" json:"id"`

	// SagaID references the Saga instance that completed the forward step.
	SagaID SagaID `db:"saga_id" json:"saga_id"`

	// Step identifies which forward Saga step completed.
	Step SagaStep `db:"step" json:"step"`

	// Sequence is a positive, monotonically increasing number scoped to SagaID.
	Sequence int64 `db:"sequence" json:"sequence"`

	// AttemptCount captures the current step-attempt counter when the forward
	// step completion was durably recorded.
	AttemptCount int `db:"attempt_count" json:"attempt_count"`

	// CompensationState tracks whether this completed forward step has been compensated.
	CompensationState SagaStepCompensationState `db:"compensation_state" json:"compensation_state"`

	// CompletedAt is when the forward step completion was durably recorded.
	CompletedAt time.Time `db:"completed_at" json:"completed_at"`

	// CompensationResolvedAt is set when CompensationState becomes a terminal
	// compensation state.
	CompensationResolvedAt *time.Time `db:"compensation_resolved_at" json:"compensation_resolved_at,omitempty"`
}

// Validate checks generic durable invariants for a forward-step completion.
func (c SagaForwardStepCompletion) Validate() error {
	const op = "model.SagaForwardStepCompletion.Validate"

	if c.ID.IsZero() {
		return fmt.Errorf("%s: %w: id is required", op, ErrInvalidSagaStepCompletion)
	}

	if c.SagaID.IsZero() {
		return fmt.Errorf("%s: %w: saga id is required", op, ErrInvalidSagaStepCompletion)
	}

	if !c.Step.IsValid() {
		return fmt.Errorf("%s: %w: step is required", op, ErrInvalidSagaStepCompletion)
	}

	if c.Sequence <= 0 {
		return fmt.Errorf("%s: %w: sequence must be positive", op, ErrInvalidSagaStepCompletion)
	}

	if c.AttemptCount <= 0 {
		return fmt.Errorf("%s: %w: attempt count must be positive", op, ErrInvalidSagaStepCompletion)
	}

	if !c.CompensationState.IsValid() {
		return fmt.Errorf(
			"%s: %w: invalid compensation state %q", op, ErrInvalidSagaStepCompletion,
			c.CompensationState,
		)
	}

	if c.CompletedAt.IsZero() {
		return fmt.Errorf("%s: %w: completed_at is required", op, ErrInvalidSagaStepCompletion)
	}

	if c.CompensationState.IsTerminal() {
		if c.CompensationResolvedAt == nil {
			return fmt.Errorf(
				"%s: %w: resolved compensation state requires compensation_resolved_at", op, ErrInvalidSagaStepCompletion,
			)
		}

		if c.CompensationResolvedAt.Before(c.CompletedAt) {
			return fmt.Errorf(
				"%s: %w: compensation_resolved_at must not precede completed_at", op, ErrInvalidSagaStepCompletion,
			)
		}
	} else if c.CompensationResolvedAt != nil {
		return fmt.Errorf(
			"%s: %w: unresolved compensation state must not have compensation_resolved_at", op, ErrInvalidSagaStepCompletion,
		)
	}

	return nil
}

// ValidateTransitionTo checks the only mutable part of a completion record: its
// compensation resolution state.
func (c SagaForwardStepCompletion) ValidateTransitionTo(next SagaForwardStepCompletion) error {
	const op = "model.SagaForwardStepCompletion.ValidateTransitionTo"

	if err := c.Validate(); err != nil {
		return err
	}

	if c.CompensationState.IsTerminal() {
		return fmt.Errorf(
			"%s: %w: resolved step completion cannot transition", op, ErrInvalidStatusTransition,
		)
	}

	if c.ID != next.ID ||
		c.SagaID != next.SagaID ||
		c.Step != next.Step ||
		c.Sequence != next.Sequence ||
		c.AttemptCount != next.AttemptCount ||
		!c.CompletedAt.Equal(next.CompletedAt) {
		return fmt.Errorf(
			"%s: %w: forward step completion identity is immutable", op, ErrInvalidSagaStepCompletion,
		)
	}

	if err := c.CompensationState.ValidateTransitionTo(next.CompensationState); err != nil {
		return err
	}

	return next.Validate()
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}
