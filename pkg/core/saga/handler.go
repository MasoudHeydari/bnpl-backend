package saga

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// StepOutcome describes the orchestration decision produced by executing a
// Saga step.
//
// A step must explicitly describe how the Saga should proceed. The execution
// infrastructure must not infer retry, compensation, reconciliation, or
// terminal behavior solely from a returned error.
type StepOutcome string

const (
	// StepOutcomeCompleted indicates that the current step completed
	// successfully and the Saga may proceed to the next step.
	StepOutcomeCompleted StepOutcome = "completed"

	// StepOutcomeWaiting indicates that execution reached a durable wait
	// boundary.
	//
	// A waiting Saga remains inactive until an external trigger or another
	// explicitly modeled mechanism makes it runnable again. This outcome does
	// not itself schedule a time-based retry.
	StepOutcomeWaiting StepOutcome = "waiting"

	// StepOutcomeRetry indicates that the current logical step should be
	// attempted again after RetryAt.
	//
	// Steps that perform external side effects must remain safe when executed
	// more than once, typically through idempotency or equivalent safeguards.
	StepOutcomeRetry StepOutcome = "retry"

	// StepOutcomeCompensate indicates that forward execution can no longer
	// continue and the Saga should enter its compensation flow.
	StepOutcomeCompensate StepOutcome = "compensate"

	// StepOutcomeReconcile indicates that the correct state cannot be safely
	// determined from the currently available information and an explicit
	// reconciliation process is required.
	StepOutcomeReconcile StepOutcome = "reconcile"

	// StepOutcomeManualReview indicates that automatic processing must stop
	// until an explicitly controlled external decision allows execution to
	// continue or terminate.
	StepOutcomeManualReview StepOutcome = "manual_review"

	// StepOutcomeFailed indicates that the Saga has reached a terminal failure
	// and no further automatic forward or compensating action is appropriate.
	StepOutcomeFailed StepOutcome = "failed"
)

// StepResult describes the orchestration result produced by executing or
// compensating a Saga step.
//
// StepResult carries process-level semantics. A returned error represents the
// technical cause associated with the execution and must not, by itself,
// determine how the Saga proceeds.
//
// Consumers must call Validate before acting on the result and must validate
// any NextStep against the owning Definition before committing a durable Saga
// transition.
type StepResult struct {
	// Outcome is the orchestration decision produced by the step.
	Outcome StepOutcome

	// NextStep identifies the next durable step to execute.
	//
	// It may refer to either a forward or compensating step depending on the
	// current Saga phase.
	//
	// A nil value means that the outcome does not immediately transition to
	// another step.
	NextStep *model.SagaStep

	// RetryAt specifies the earliest time at which the current logical step may
	// be attempted again.
	//
	// It is meaningful only when Outcome is StepOutcomeRetry.
	RetryAt *time.Time

	// Reason contains a stable internal explanation for the resulting
	// transition.
	//
	// It is intended for diagnostics and audit context. Persistence of the reason
	// is an application concern; it must not be treated as a user-facing message.
	Reason *string
}

// Step represents one durable operation in a Saga.
//
// Step is intentionally limited to durable identity. Execution is segregated
// by transaction semantics so the orchestrator can preserve local atomicity
// without holding a database transaction open across external calls.
type Step interface {
	// Name returns the stable identifier of the step.
	//
	// Step names may be persisted as part of durable Saga state and therefore
	// must remain backward-compatible once they are used in persisted data.
	Name() model.SagaStep
}

// TransactionalStep is a step whose operation must participate in the same
// local transaction as the resulting Saga state transition.
//
// Implementations receive the transaction explicitly and should bind every
// related repository operation to it. They must not perform potentially slow
// external network calls while the transaction is open.
type TransactionalStep interface {
	// Step provides the durable step identity.
	Step

	// ExecuteTx runs the step inside the supplied local transaction.
	ExecuteTx(
		ctx context.Context,
		tx repo.Tx,
		instance model.SagaInstance,
	) (StepResult, error)
}

// ExternalStep is a step that executes outside a local database transaction.
//
// ExternalStep is intended for operations that may block on another process or
// system. Because a process can crash after the external effect succeeds but
// before the Saga result is persisted, implementations must be safe to invoke
// more than once, typically through a stable idempotency key or an equivalent
// deduplication mechanism. ExternalStep implementations should not commit local
// durable business state directly; local writes that must be atomic with the
// Saga transition belong in a TransactionalStep.
type ExternalStep interface {
	// Step provides the durable step identity.
	Step

	// Execute runs the step outside a local database transaction.
	Execute(
		ctx context.Context,
		instance model.SagaInstance,
	) (StepResult, error)
}

// TransactionalCompensatableStep provides direct compensation that must be
// committed atomically with the corresponding Saga compensation transition.
type TransactionalCompensatableStep interface {
	// TransactionalStep provides forward execution inside a local transaction.
	TransactionalStep

	// CompensateTx reverses the step inside the supplied local transaction.
	CompensateTx(
		ctx context.Context,
		tx repo.Tx,
		instance model.SagaInstance,
	) (StepResult, error)
}

// ExternalCompensatableStep provides direct compensation that executes outside
// a local database transaction and therefore carries the same retry-safety and
// idempotency requirements as ExternalStep.
type ExternalCompensatableStep interface {
	// ExternalStep provides forward execution outside a local transaction.
	ExternalStep

	// Compensate reverses the step outside a local database transaction.
	Compensate(
		ctx context.Context,
		instance model.SagaInstance,
	) (StepResult, error)
}

// CompensationMode describes how the effect of a completed forward step is
// compensated.
type CompensationMode string

const (
	// CompensationModeNone explicitly declares that the completed forward step
	// does not require a compensating operation.
	//
	// This is different from omitting a compensation policy. An omitted policy
	// is treated as an incomplete definition when compensation is required.
	CompensationModeNone CompensationMode = "none"

	// CompensationModeDirect indicates that compensation is performed by
	// invoking the direct compensation contract of the original step.
	CompensationModeDirect CompensationMode = "direct"

	// CompensationModeWorkflow indicates that compensation is represented by a
	// separate explicit sequence or graph of Saga steps.
	//
	// This mode is appropriate when compensation requires multiple operations,
	// durable waits, external interactions, or other non-trivial processing.
	CompensationModeWorkflow CompensationMode = "workflow"
)

// Compensation describes how a previously completed forward step is
// compensated.
type Compensation struct {
	// Mode selects how compensation is performed for a completed forward step.
	Mode CompensationMode

	// EntryStep identifies the first step of an explicit compensation workflow.
	// It must be present only for CompensationModeWorkflow.
	EntryStep *model.SagaStep

	// WorkflowSteps contains every step that belongs to this explicit
	// compensation workflow. The entry and terminal steps must be members.
	WorkflowSteps []model.SagaStep

	// WorkflowTerminalSteps identifies the steps whose successful completion
	// finishes this compensation workflow.
	WorkflowTerminalSteps []model.SagaStep
}

// ContainsWorkflowStep reports whether step belongs to an explicit
// compensation workflow.
func (c Compensation) ContainsWorkflowStep(step model.SagaStep) bool {
	for _, candidate := range c.WorkflowSteps {
		if candidate == step {
			return true
		}
	}

	return false
}

// IsWorkflowTerminalStep reports whether step completes an explicit
// compensation workflow.
func (c Compensation) IsWorkflowTerminalStep(step model.SagaStep) bool {
	for _, candidate := range c.WorkflowTerminalSteps {
		if candidate == step {
			return true
		}
	}

	return false
}

// Definition describes the static structure and allowed execution graph of one
// Saga type.
//
// A Definition declares which steps belong to the Saga, where forward execution
// starts, which compensation strategy applies to completed forward steps, and
// which transitions are valid.
//
// A Definition must not contain infrastructure-specific behavior. Runtime
// decisions such as retry, compensation, reconciliation, or manual intervention
// are produced by executing individual Steps.
type Definition interface {
	// Type returns the stable identifier of the Saga type represented by this
	// definition.
	Type() model.SagaType

	// Version returns the immutable workflow-definition version represented by
	// this definition. In-flight Saga instances persist this value so they can
	// continue using the same graph across application deployments.
	Version() model.SagaDefinitionVersion

	// InitialStep returns the first step used when forward execution of a new
	// Saga begins.
	InitialStep() model.SagaStep

	// IsForwardTerminalStep reports whether successful completion of step with no
	// NextStep finishes the forward Saga.
	IsForwardTerminalStep(step model.SagaStep) bool

	// Step resolves a durable step identifier to its implementation.
	//
	// Returning false indicates that the step is not part of this Saga
	// definition. Callers must stop execution rather than attempting to infer or
	// substitute another step.
	Step(name model.SagaStep) (Step, bool)

	// CompensationFor describes how the effect of a successfully completed
	// forward step should be compensated.
	//
	// Returning false means that no compensation policy has been declared for the
	// supplied step. This is a definition gap and must not be interpreted as
	// "no compensation required". Use CompensationModeNone to declare that case
	// explicitly.
	//
	// Compensation may be unnecessary, direct through a compensatable step
	// contract, or an explicit compensation workflow.
	CompensationFor(
		completedStep model.SagaStep,
	) (Compensation, bool)

	// CanTransition reports whether a transition between two durable steps is
	// valid for the supplied Saga phase.
	//
	// Every transition returned by a Step should be validated against the
	// Definition before it is persisted. This prevents invalid, stale, or buggy
	// step implementations from moving a Saga outside its declared execution
	// graph.
	//
	// Forward and compensating phases may define completely different graphs.
	CanTransition(
		phase model.SagaPhase,
		from model.SagaStep,
		to model.SagaStep,
	) bool
}
