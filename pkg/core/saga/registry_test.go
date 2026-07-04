package saga_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func TestRegistry(t *testing.T) {
	t.Parallel()

	initial := model.SagaStep("start")
	step := newNamedStep(t, initial)
	definition := newValidDefinition(t, "process-a", 1, initial, map[model.SagaStep]saga.Step{
		initial: step,
	}, nil, nil)

	registry, err := saga.NewRegistry(definition)
	require.NoError(t, err)

	resolved, ok := registry.Definition("process-a", 1)
	require.True(t, ok)
	assert.Same(t, definition, resolved)

	_, err = registry.RequireDefinition("missing", 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, saga.ErrDefinitionNotFound)
}

func TestRegistry_RejectsDuplicateTypes(t *testing.T) {
	t.Parallel()

	initialA := model.SagaStep("start-a")
	initialB := model.SagaStep("start-b")
	definitionA := newValidDefinition(t, "process", 1, initialA, map[model.SagaStep]saga.Step{
		initialA: newNamedStep(t, initialA),
	}, nil, nil)
	definitionB := newValidDefinition(t, "process", 1, initialB, map[model.SagaStep]saga.Step{
		initialB: newNamedStep(t, initialB),
	}, nil, nil)

	_, err := saga.NewRegistry(definitionA, definitionB)
	require.Error(t, err)
	assert.ErrorIs(t, err, saga.ErrInvalidDefinition)
}

func TestRegistry_AllowsMultipleVersionsOfSameType(t *testing.T) {
	t.Parallel()

	initialV1 := model.SagaStep("start-v1")
	initialV2 := model.SagaStep("start-v2")
	definitionV1 := newValidDefinition(t, "process", 1, initialV1, map[model.SagaStep]saga.Step{
		initialV1: newNamedStep(t, initialV1),
	}, nil, nil)
	definitionV2 := newValidDefinition(t, "process", 2, initialV2, map[model.SagaStep]saga.Step{
		initialV2: newNamedStep(t, initialV2),
	}, nil, nil)

	registry, err := saga.NewRegistry(definitionV1, definitionV2)
	require.NoError(t, err)

	resolvedV1, ok := registry.Definition("process", 1)
	require.True(t, ok)
	assert.Same(t, definitionV1, resolvedV1)

	resolvedV2, ok := registry.Definition("process", 2)
	require.True(t, ok)
	assert.Same(t, definitionV2, resolvedV2)
}

func newNamedStep(t *testing.T, name model.SagaStep) *mock.Step {
	t.Helper()

	step := mock.NewStep(t)
	step.EXPECT().Name().Return(name)
	return step
}

func newValidDefinition(
	t *testing.T,
	typ model.SagaType,
	version model.SagaDefinitionVersion,
	initial model.SagaStep,
	steps map[model.SagaStep]saga.Step,
	compensations map[model.SagaStep]saga.Compensation,
	canTransition func(phase model.SagaPhase, from, to model.SagaStep) bool,
) *mock.Definition {
	t.Helper()

	definition := mock.NewDefinition(t)
	// Type/Version/InitialStep are only required by ValidateDefinition /
	// NewRegistry paths; ResolveCompensation does not call them.
	definition.EXPECT().Type().Return(typ).Maybe()
	definition.EXPECT().Version().Return(version).Maybe()
	definition.EXPECT().InitialStep().Return(initial).Maybe()
	definition.EXPECT().Step(testifymock.Anything).RunAndReturn(
		func(name model.SagaStep) (saga.Step, bool) {
			step, ok := steps[name]
			return step, ok
		},
	)

	if compensations != nil {
		definition.EXPECT().CompensationFor(testifymock.Anything).RunAndReturn(
			func(completedStep model.SagaStep) (saga.Compensation, bool) {
				compensation, ok := compensations[completedStep]
				return compensation, ok
			},
		)
	}

	if canTransition != nil {
		definition.EXPECT().CanTransition(
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
		).RunAndReturn(canTransition)
	}

	return definition
}
