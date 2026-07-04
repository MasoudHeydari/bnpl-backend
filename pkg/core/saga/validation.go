package saga

import (
	"errors"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var (
	// ErrInvalidStepResult is returned when a step produces an internally
	// inconsistent orchestration result.
	ErrInvalidStepResult = errors.New("invalid saga step result")

	// ErrInvalidCompensation is returned when a compensation declaration is
	// incomplete or inconsistent with its declared mode.
	ErrInvalidCompensation = errors.New("invalid saga compensation")

	// ErrInvalidDefinition is returned when a Saga definition does not satisfy
	// the generic structural requirements expected by the orchestration layer.
	ErrInvalidDefinition = errors.New("invalid saga definition")

	// ErrStepNotFound is returned when a definition cannot resolve a declared
	// durable step identifier.
	ErrStepNotFound = errors.New("saga step not found")

	// ErrCompensationNotDefined is returned when a completed forward step has no
	// explicit compensation policy in its Saga definition.
	ErrCompensationNotDefined = errors.New("saga compensation not defined")
)

// IsValid reports whether the outcome is recognized by the generic Saga
// orchestration contract.
func (o StepOutcome) IsValid() bool {
	switch o {
	case StepOutcomeCompleted,
		StepOutcomeWaiting,
		StepOutcomeRetry,
		StepOutcomeCompensate,
		StepOutcomeReconcile,
		StepOutcomeManualReview,
		StepOutcomeFailed:
		return true

	default:
		return false
	}
}

// Validate checks the generic field-level invariants of a StepResult.
//
// Workflow-specific transition validity is intentionally not checked here. A
// caller must additionally validate NextStep through Definition.CanTransition
// before committing a durable step change.
func (r StepResult) Validate() error {
	const op = "saga.StepResult.Validate"

	if !r.Outcome.IsValid() {
		return fmt.Errorf("%s: %w: unknown outcome %q", op, ErrInvalidStepResult, r.Outcome)
	}

	if r.NextStep != nil && !r.NextStep.IsValid() {
		return fmt.Errorf("%s: %w: next step must not be blank", op, ErrInvalidStepResult)
	}

	if r.RetryAt != nil && r.RetryAt.IsZero() {
		return fmt.Errorf("%s: %w: retry_at must not be zero", op, ErrInvalidStepResult)
	}

	if r.Reason != nil && strings.TrimSpace(*r.Reason) == "" {
		return fmt.Errorf("%s: %w: reason must not be blank", op, ErrInvalidStepResult)
	}

	switch r.Outcome {
	case StepOutcomeCompleted:
		if r.RetryAt != nil {
			return fmt.Errorf("%s: %w: completed outcome must not set retry_at", op, ErrInvalidStepResult)
		}

	case StepOutcomeWaiting:
		if r.NextStep != nil || r.RetryAt != nil {
			return fmt.Errorf(
				"%s: %w: waiting outcome must not set next_step or retry_at", op, ErrInvalidStepResult,
			)
		}

	case StepOutcomeRetry:
		if r.RetryAt == nil {
			return fmt.Errorf("%s: %w: retry outcome requires retry_at", op, ErrInvalidStepResult)
		}

		if r.NextStep != nil {
			return fmt.Errorf("%s: %w: retry outcome must not change step", op, ErrInvalidStepResult)
		}

	case StepOutcomeCompensate,
		StepOutcomeReconcile,
		StepOutcomeManualReview,
		StepOutcomeFailed:
		if r.NextStep != nil || r.RetryAt != nil {
			return fmt.Errorf(
				"%s: %w: outcome %q must not set next_step or retry_at", op, ErrInvalidStepResult,
				r.Outcome,
			)
		}
	}

	return nil
}

// IsValid reports whether the compensation mode is recognized by the generic
// Saga orchestration contract.
func (m CompensationMode) IsValid() bool {
	switch m {
	case CompensationModeNone,
		CompensationModeDirect,
		CompensationModeWorkflow:
		return true

	default:
		return false
	}
}

// Validate checks the generic structural invariants of a compensation policy.
func (c Compensation) Validate() error {
	const op = "saga.Compensation.Validate"

	if !c.Mode.IsValid() {
		return fmt.Errorf("%s: %w: unknown mode %q", op, ErrInvalidCompensation, c.Mode)
	}

	switch c.Mode {
	case CompensationModeNone, CompensationModeDirect:
		if c.EntryStep != nil || len(c.WorkflowSteps) != 0 || len(c.WorkflowTerminalSteps) != 0 {
			return fmt.Errorf(
				"%s: %w: mode %q must not define workflow metadata", op, ErrInvalidCompensation,
				c.Mode,
			)
		}

	case CompensationModeWorkflow:
		if c.EntryStep == nil || !c.EntryStep.IsValid() {
			return fmt.Errorf(
				"%s: %w: workflow mode requires a valid entry step", op, ErrInvalidCompensation,
			)
		}

		if len(c.WorkflowSteps) == 0 {
			return fmt.Errorf("%s: %w: workflow mode requires workflow steps", op, ErrInvalidCompensation)
		}

		if len(c.WorkflowTerminalSteps) == 0 {
			return fmt.Errorf("%s: %w: workflow mode requires terminal steps", op, ErrInvalidCompensation)
		}

		seen := make(map[model.SagaStep]struct{}, len(c.WorkflowSteps))
		for _, step := range c.WorkflowSteps {
			if !step.IsValid() {
				return fmt.Errorf("%s: %w: workflow step must not be blank", op, ErrInvalidCompensation)
			}

			if _, exists := seen[step]; exists {
				return fmt.Errorf("%s: %w: duplicate workflow step %q", op, ErrInvalidCompensation, step)
			}

			seen[step] = struct{}{}
		}

		if !c.ContainsWorkflowStep(*c.EntryStep) {
			return fmt.Errorf("%s: %w: workflow entry step must belong to workflow", op, ErrInvalidCompensation)
		}

		terminalSeen := make(map[model.SagaStep]struct{}, len(c.WorkflowTerminalSteps))
		for _, step := range c.WorkflowTerminalSteps {
			if !step.IsValid() {
				return fmt.Errorf("%s: %w: workflow terminal step must not be blank", op, ErrInvalidCompensation)
			}

			if !c.ContainsWorkflowStep(step) {
				return fmt.Errorf("%s: %w: terminal step %q must belong to workflow", op, ErrInvalidCompensation, step)
			}

			if _, exists := terminalSeen[step]; exists {
				return fmt.Errorf("%s: %w: duplicate terminal step %q", op, ErrInvalidCompensation, step)
			}

			terminalSeen[step] = struct{}{}
		}
	}

	return nil
}

// ValidateDefinition performs the generic validation that can be checked
// without enumerating a definition's complete workflow graph.
//
// In addition to this validation, every resolved transition and compensation
// policy must be validated at the point where it is used.
func ValidateDefinition(def Definition) error {
	const op = "saga.ValidateDefinition"

	if def == nil {
		return fmt.Errorf("%s: %w: definition is required", op, ErrInvalidDefinition)
	}

	if !def.Type().IsValid() {
		return fmt.Errorf("%s: %w: saga type is required", op, ErrInvalidDefinition)
	}

	if !def.Version().IsValid() {
		return fmt.Errorf("%s: %w: definition version is required", op, ErrInvalidDefinition)
	}

	if !def.InitialStep().IsValid() {
		return fmt.Errorf("%s: %w: initial step is required", op, ErrInvalidDefinition)
	}

	if _, err := ResolveStep(def, def.InitialStep()); err != nil {
		return fmt.Errorf("%s: %w: %v", op, ErrInvalidDefinition, err)
	}

	return nil
}

// ResolveStep resolves and validates a step declared by a Saga definition.
//
// The returned implementation must report the same stable name that was used
// to resolve it. This prevents accidental registry aliases from silently
// changing the durable meaning of a persisted step identifier.
func ResolveStep(def Definition, name model.SagaStep) (Step, error) {
	const op = "saga.ResolveStep"

	if def == nil {
		return nil, fmt.Errorf("%s: %w: definition is required", op, ErrInvalidDefinition)
	}

	if !name.IsValid() {
		return nil, fmt.Errorf("%s: %w: step name is required", op, ErrStepNotFound)
	}

	step, ok := def.Step(name)
	if !ok || step == nil {
		return nil, fmt.Errorf("%s: %w: %q", op, ErrStepNotFound, name)
	}

	if step.Name() != name {
		return nil, fmt.Errorf(
			"%s: %w: resolved step %q reports name %q", op, ErrInvalidDefinition,
			name,
			step.Name(),
		)
	}

	return step, nil
}

// ResolveCompensation resolves and validates the explicit compensation policy
// for a completed forward step.
//
// CompensationModeNone is a valid and explicit declaration that no compensating
// action is required. A missing policy is different: it indicates that the
// definition is incomplete for compensation and must not be guessed by the
// execution infrastructure.
func ResolveCompensation(
	def Definition,
	completedStep model.SagaStep,
) (Compensation, error) {
	const op = "saga.ResolveCompensation"

	if def == nil {
		return Compensation{}, fmt.Errorf("%s: %w: definition is required", op, ErrInvalidDefinition)
	}

	if !completedStep.IsValid() {
		return Compensation{}, fmt.Errorf(
			"%s: %w: completed step is required", op, ErrCompensationNotDefined,
		)
	}

	if _, err := ResolveStep(def, completedStep); err != nil {
		return Compensation{}, err
	}

	compensation, ok := def.CompensationFor(completedStep)
	if !ok {
		return Compensation{}, fmt.Errorf(
			"%s: %w: step %q", op, ErrCompensationNotDefined,
			completedStep,
		)
	}

	if err := compensation.Validate(); err != nil {
		return Compensation{}, err
	}

	switch compensation.Mode {
	case CompensationModeNone:
		return compensation, nil

	case CompensationModeDirect:
		step, err := ResolveStep(def, completedStep)
		if err != nil {
			return Compensation{}, err
		}

		if _, txOK := step.(TransactionalCompensatableStep); !txOK {
			if _, externalOK := step.(ExternalCompensatableStep); !externalOK {
				return Compensation{}, fmt.Errorf(
					"%s: %w: step %q declares direct compensation but does not implement a compensatable step contract", op, ErrInvalidCompensation,
					completedStep,
				)
			}
		}

	case CompensationModeWorkflow:
		for _, stepName := range compensation.WorkflowSteps {
			if _, err := ResolveStep(def, stepName); err != nil {
				return Compensation{}, fmt.Errorf("%s: %w: %v", op, ErrInvalidCompensation, err)
			}
		}
	}

	return compensation, nil
}

// ValidateForPhase checks StepResult invariants that depend on the current Saga
// execution phase.
//
// Starting another compensation cycle while already compensating is rejected.
// Recovery inside a compensation workflow must be modeled explicitly through
// retry, reconciliation, manual review, failure, or compensation-phase step
// transitions rather than recursive compensation.
func (r StepResult) ValidateForPhase(phase model.SagaPhase) error {
	const op = "saga.StepResult.ValidateForPhase"

	if err := r.Validate(); err != nil {
		return err
	}

	if !phase.IsValid() {
		return fmt.Errorf("%s: %w: invalid saga phase %q", op, ErrInvalidStepResult, phase)
	}

	if phase == model.CompensatingSagaPhase && r.Outcome == StepOutcomeCompensate {
		return fmt.Errorf(
			"%s: %w: compensation outcome is not valid while already compensating", op, ErrInvalidStepResult,
		)
	}

	return nil
}

// ValidateStepResult validates a step result against the exact durable Saga
// definition and current instance that produced it.
//
// This helper centralizes generic checks that must happen before a resulting
// durable transition is committed. It does not apply workflow-specific state
// changes or persistence.
func ValidateStepResult(
	def Definition,
	instance model.SagaInstance,
	result StepResult,
) error {
	const op = "saga.ValidateStepResult"

	if err := ValidateDefinition(def); err != nil {
		return err
	}

	if def.Type() != instance.Type {
		return fmt.Errorf(
			"%s: %w: definition type %q does not match instance type %q", op, ErrInvalidDefinition,
			def.Type(),
			instance.Type,
		)
	}

	if def.Version() != instance.DefinitionVersion {
		return fmt.Errorf(
			"%s: %w: definition version %d does not match instance version %d", op, ErrInvalidDefinition,
			def.Version(),
			instance.DefinitionVersion,
		)
	}

	if _, err := ResolveStep(def, instance.CurrentStep); err != nil {
		return err
	}

	if err := result.ValidateForPhase(instance.Phase); err != nil {
		return err
	}

	if result.NextStep == nil {
		return nil
	}

	if _, err := ResolveStep(def, *result.NextStep); err != nil {
		return err
	}

	if !def.CanTransition(instance.Phase, instance.CurrentStep, *result.NextStep) {
		return fmt.Errorf(
			"%s: %w: transition from %q to %q is not allowed in phase %q", op, ErrInvalidStepResult,
			instance.CurrentStep,
			*result.NextStep,
			instance.Phase,
		)
	}

	return nil
}
