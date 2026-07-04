package saga_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func TestStepResultValidate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	next := model.SagaStep("next")

	tests := []struct {
		name    string
		result  saga.StepResult
		wantErr bool
	}{
		{
			name:   "completed may advance",
			result: saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next},
		},
		{
			name:   "completed may finish path",
			result: saga.StepResult{Outcome: saga.StepOutcomeCompleted},
		},
		{
			name:   "waiting has no scheduling metadata",
			result: saga.StepResult{Outcome: saga.StepOutcomeWaiting},
		},
		{
			name:   "retry requires retry time",
			result: saga.StepResult{Outcome: saga.StepOutcomeRetry, RetryAt: &now},
		},
		{
			name:    "retry without retry time is invalid",
			result:  saga.StepResult{Outcome: saga.StepOutcomeRetry},
			wantErr: true,
		},
		{
			name:    "retry cannot change step",
			result:  saga.StepResult{Outcome: saga.StepOutcomeRetry, RetryAt: &now, NextStep: &next},
			wantErr: true,
		},
		{
			name:    "compensation entry is definition owned",
			result:  saga.StepResult{Outcome: saga.StepOutcomeCompensate, NextStep: &next},
			wantErr: true,
		},
		{
			name:    "unknown outcome is invalid",
			result:  saga.StepResult{Outcome: saga.StepOutcome("unknown")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.result.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, saga.ErrInvalidStepResult)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestCompensationValidate(t *testing.T) {
	t.Parallel()

	entry := model.SagaStep("compensate_resource")

	t.Run("none is explicit and valid", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, (saga.Compensation{Mode: saga.CompensationModeNone}).Validate())
	})

	t.Run("direct must not have entry step", func(t *testing.T) {
		t.Parallel()
		err := (saga.Compensation{Mode: saga.CompensationModeDirect, EntryStep: &entry}).Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, saga.ErrInvalidCompensation)
	})

	t.Run("workflow requires entry step", func(t *testing.T) {
		t.Parallel()
		err := (saga.Compensation{Mode: saga.CompensationModeWorkflow}).Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, saga.ErrInvalidCompensation)
	})

	t.Run("workflow with declared graph is valid", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, (saga.Compensation{
			Mode:                  saga.CompensationModeWorkflow,
			EntryStep:             &entry,
			WorkflowSteps:         []model.SagaStep{entry},
			WorkflowTerminalSteps: []model.SagaStep{entry},
		}).Validate())
	})
}

func TestResolveCompensation(t *testing.T) {
	t.Parallel()

	t.Run("distinguishes explicit none from missing policy", func(t *testing.T) {
		t.Parallel()

		forward := model.SagaStep("forward")
		withoutPolicy := model.SagaStep("without-policy")
		steps := map[model.SagaStep]saga.Step{
			forward:       newNamedStep(t, forward),
			withoutPolicy: newNamedStep(t, withoutPolicy),
		}
		def := newValidDefinition(t, "process", 1, forward, steps, map[model.SagaStep]saga.Compensation{
			forward: {Mode: saga.CompensationModeNone},
		}, nil)

		compensation, err := saga.ResolveCompensation(def, forward)
		require.NoError(t, err)
		assert.Equal(t, saga.CompensationModeNone, compensation.Mode)

		_, err = saga.ResolveCompensation(def, withoutPolicy)
		require.Error(t, err)
		assert.ErrorIs(t, err, saga.ErrCompensationNotDefined)
	})

	t.Run("direct compensation requires compensatable step", func(t *testing.T) {
		t.Parallel()

		forward := model.SagaStep("forward")
		def := newValidDefinition(t, "process", 1, forward, map[model.SagaStep]saga.Step{
			forward: newNamedStep(t, forward),
		}, map[model.SagaStep]saga.Compensation{
			forward: {Mode: saga.CompensationModeDirect},
		}, nil)

		_, err := saga.ResolveCompensation(def, forward)
		require.Error(t, err)
		assert.ErrorIs(t, err, saga.ErrInvalidCompensation)
	})

	t.Run("direct compensation accepts compensatable step", func(t *testing.T) {
		t.Parallel()

		forward := model.SagaStep("forward")
		step := mock.NewExternalCompensatableStep(t)
		step.EXPECT().Name().Return(forward)

		def := newValidDefinition(t, "process", 1, forward, map[model.SagaStep]saga.Step{
			forward: step,
		}, map[model.SagaStep]saga.Compensation{
			forward: {Mode: saga.CompensationModeDirect},
		}, nil)

		compensation, err := saga.ResolveCompensation(def, forward)
		require.NoError(t, err)
		assert.Equal(t, saga.CompensationModeDirect, compensation.Mode)
	})

	t.Run("workflow entry must resolve", func(t *testing.T) {
		t.Parallel()

		forward := model.SagaStep("forward")
		missing := model.SagaStep("missing")
		def := newValidDefinition(t, "process", 1, forward, map[model.SagaStep]saga.Step{
			forward: newNamedStep(t, forward),
		}, map[model.SagaStep]saga.Compensation{
			forward: {
				Mode:                  saga.CompensationModeWorkflow,
				EntryStep:             &missing,
				WorkflowSteps:         []model.SagaStep{missing},
				WorkflowTerminalSteps: []model.SagaStep{missing},
			},
		}, nil)

		_, err := saga.ResolveCompensation(def, forward)
		require.Error(t, err)
		assert.ErrorIs(t, err, saga.ErrInvalidCompensation)
	})
}

func TestValidateDefinition(t *testing.T) {
	t.Parallel()

	initial := model.SagaStep("start")
	def := newValidDefinition(t, "process", 1, initial, map[model.SagaStep]saga.Step{
		initial: newNamedStep(t, initial),
	}, nil, nil)
	require.NoError(t, saga.ValidateDefinition(def))

	mismatched := mock.NewStep(t)
	mismatched.EXPECT().Name().Return(model.SagaStep("different"))
	bad := newValidDefinition(t, "process", 1, initial, map[model.SagaStep]saga.Step{
		initial: mismatched,
	}, nil, nil)
	err := saga.ValidateDefinition(bad)
	require.Error(t, err)
	assert.ErrorIs(t, err, saga.ErrInvalidDefinition)
}

func TestValidateStepResult(t *testing.T) {
	t.Parallel()

	start := model.SagaStep("start")
	next := model.SagaStep("next")
	notAllowed := model.SagaStep("not-allowed")
	steps := map[model.SagaStep]saga.Step{
		start:      newNamedStep(t, start),
		next:       newNamedStep(t, next),
		notAllowed: newNamedStep(t, notAllowed),
	}
	def := newValidDefinition(
		t,
		"process",
		1,
		start,
		steps,
		nil,
		func(phase model.SagaPhase, from, to model.SagaStep) bool {
			return phase == model.ForwardSagaPhase && from == start && to == next
		},
	)

	instance := model.SagaInstance{
		Type:              "process",
		DefinitionVersion: 1,
		Phase:             model.ForwardSagaPhase,
		CurrentStep:       start,
	}
	result := saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next}
	require.NoError(t, saga.ValidateStepResult(def, instance, result))

	result.NextStep = &notAllowed
	err := saga.ValidateStepResult(def, instance, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, saga.ErrInvalidStepResult)
}

func TestStepResultValidateForPhase_RejectsRecursiveCompensation(t *testing.T) {
	t.Parallel()

	result := saga.StepResult{Outcome: saga.StepOutcomeCompensate}
	err := result.ValidateForPhase(model.CompensatingSagaPhase)
	require.Error(t, err)
	assert.ErrorIs(t, err, saga.ErrInvalidStepResult)
}
