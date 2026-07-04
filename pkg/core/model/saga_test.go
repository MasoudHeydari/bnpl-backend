package model

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSagaPhase_Transitions(t *testing.T) {
	t.Parallel()

	t.Run("forward may enter compensation", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ForwardSagaPhase.ValidateTransitionTo(CompensatingSagaPhase))
	})

	t.Run("compensation cannot return to forward", func(t *testing.T) {
		t.Parallel()
		err := CompensatingSagaPhase.ValidateTransitionTo(ForwardSagaPhase)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
	})
}

func TestSagaStatus_Transitions(t *testing.T) {
	t.Parallel()

	t.Run("running may wait", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, RunningSagaStatus.ValidateTransitionTo(WaitingSagaStatus))
	})

	t.Run("waiting may resume", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, WaitingSagaStatus.ValidateTransitionTo(RunningSagaStatus))
	})

	t.Run("reconciliation may resume", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ReconciliationRequiredSagaStatus.ValidateTransitionTo(RunningSagaStatus))
	})

	t.Run("completed is terminal", func(t *testing.T) {
		t.Parallel()
		assert.True(t, CompletedSagaStatus.IsTerminal())
		err := CompletedSagaStatus.ValidateTransitionTo(RunningSagaStatus)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
	})

	t.Run("compensated is terminal", func(t *testing.T) {
		t.Parallel()
		assert.True(t, CompensatedSagaStatus.IsTerminal())
		err := CompensatedSagaStatus.ValidateTransitionTo(RunningSagaStatus)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
	})
}

func TestSagaInstance_IsDue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)

	t.Run("running without delay or lease is due", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		assert.True(t, instance.IsDue(now))
	})

	t.Run("future next attempt is not due", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		nextAttemptAt := now.Add(time.Minute)
		instance.NextAttemptAt = &nextAttemptAt
		assert.False(t, instance.IsDue(now))
	})

	t.Run("active lease is not due", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		leaseUntil := now.Add(time.Minute)
		workerID := "worker-a"
		instance.LeaseUntil = &leaseUntil
		instance.LockedBy = &workerID
		assert.False(t, instance.IsDue(now))
	})

	t.Run("expired lease is due", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		leaseUntil := now.Add(-time.Second)
		workerID := "worker-a"
		instance.LeaseUntil = &leaseUntil
		instance.LockedBy = &workerID
		assert.True(t, instance.IsDue(now))
	})

	t.Run("waiting is never due", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		instance.Status = WaitingSagaStatus
		assert.False(t, instance.IsDue(now))
	})
}

func TestSagaInstance_Validate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)

	t.Run("valid running saga", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validSagaInstance(now).Validate())
	})

	t.Run("rejects blank business key", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		instance.BusinessKey = SagaBusinessKey("   ")
		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("rejects partial subject identity", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		subjectType := "contract"
		instance.SubjectType = &subjectType
		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("rejects partial worker lease", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		leaseUntil := now.Add(time.Minute)
		instance.LeaseUntil = &leaseUntil
		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("rejects retry schedule while waiting", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		instance.Status = WaitingSagaStatus
		nextAttemptAt := now.Add(time.Minute)
		instance.NextAttemptAt = &nextAttemptAt
		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("compensated saga requires compensating phase", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		finishedAt := now.Add(time.Second)
		instance.Status = CompensatedSagaStatus
		instance.FinishedAt = &finishedAt
		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("valid compensated saga", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		finishedAt := now.Add(time.Second)
		instance.Phase = CompensatingSagaPhase
		instance.Status = CompensatedSagaStatus
		instance.CurrentStep = SagaStep("cancel_contract")
		instance.UpdatedAt = finishedAt
		instance.FinishedAt = &finishedAt
		require.NoError(t, instance.Validate())
	})

	t.Run("terminal saga requires finished timestamp", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		instance.Status = CompletedSagaStatus
		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("non terminal saga rejects finished timestamp", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		finishedAt := now.Add(time.Second)
		instance.FinishedAt = &finishedAt
		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("requires definition version", func(t *testing.T) {
		t.Parallel()
		instance := validSagaInstance(now)
		instance.DefinitionVersion = 0

		err := instance.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})
}

func TestSagaInstance_ValidateTransitionTo(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)

	t.Run("allows forward saga to enter compensation", func(t *testing.T) {
		t.Parallel()
		current := validSagaInstance(now)
		next := current
		next.Phase = CompensatingSagaPhase
		next.CurrentStep = SagaStep("cancel_contract")
		next.UpdatedAt = now.Add(time.Second)

		require.NoError(t, current.ValidateTransitionTo(next))
	})

	t.Run("rejects immutable identity changes", func(t *testing.T) {
		t.Parallel()
		current := validSagaInstance(now)
		next := current
		next.BusinessKey = SagaBusinessKey("another-key")

		err := current.ValidateTransitionTo(next)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidSagaInstance))
	})

	t.Run("rejects terminal status reversal", func(t *testing.T) {
		t.Parallel()
		finishedAt := now.Add(time.Second)
		current := validSagaInstance(now)
		current.Status = CompletedSagaStatus
		current.FinishedAt = &finishedAt
		current.UpdatedAt = finishedAt

		next := current
		next.Status = RunningSagaStatus
		next.FinishedAt = nil
		next.UpdatedAt = finishedAt.Add(time.Second)

		err := current.ValidateTransitionTo(next)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
	})

	t.Run("rejects terminal mutation", func(t *testing.T) {
		t.Parallel()
		finishedAt := now.Add(time.Second)
		current := validSagaInstance(now)
		current.Status = CompletedSagaStatus
		current.FinishedAt = &finishedAt
		current.UpdatedAt = finishedAt

		next := current
		next.UpdatedAt = finishedAt.Add(time.Second)

		err := current.ValidateTransitionTo(next)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
	})

	t.Run("rejects immutable metadata changes", func(t *testing.T) {
		t.Parallel()
		current := validSagaInstance(now)
		correlationID := "correlation-a"
		current.CorrelationID = &correlationID

		next := current
		otherCorrelationID := "correlation-b"
		next.CorrelationID = &otherCorrelationID
		next.UpdatedAt = now.Add(time.Second)

		err := current.ValidateTransitionTo(next)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})

	t.Run("requires attempt reset on step change", func(t *testing.T) {
		t.Parallel()
		current := validSagaInstance(now)
		current.StepAttemptCount = 2

		next := current
		next.CurrentStep = SagaStep("next_step")
		next.StepAttemptCount = 2
		next.UpdatedAt = now.Add(time.Second)

		err := current.ValidateTransitionTo(next)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)

		next.StepAttemptCount = 0
		require.NoError(t, current.ValidateTransitionTo(next))
	})

	t.Run("requires attempt reset on phase change", func(t *testing.T) {
		t.Parallel()
		current := validSagaInstance(now)
		current.StepAttemptCount = 3

		next := current
		next.Phase = CompensatingSagaPhase
		next.StepAttemptCount = 3
		next.UpdatedAt = now.Add(time.Second)

		err := current.ValidateTransitionTo(next)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)

		next.StepAttemptCount = 0
		require.NoError(t, current.ValidateTransitionTo(next))
	})

	t.Run("rejects definition version change", func(t *testing.T) {
		t.Parallel()
		current := validSagaInstance(now)
		next := current
		next.DefinitionVersion = 2
		next.UpdatedAt = now.Add(time.Second)

		err := current.ValidateTransitionTo(next)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSagaInstance)
	})
}

func TestSagaForwardStepCompletion_Validate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	completion := validSagaForwardStepCompletion(now)
	require.NoError(t, completion.Validate())

	completion.AttemptCount = 0
	err := completion.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSagaStepCompletion)
}

func TestSagaForwardStepCompletion_CompensationTransition(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	current := validSagaForwardStepCompletion(now)
	resolvedAt := now.Add(time.Second)

	next := current
	next.CompensationState = SagaStepCompensated
	next.CompensationResolvedAt = &resolvedAt

	require.NoError(t, current.ValidateTransitionTo(next))

	later := next
	later.CompensationState = SagaStepCompensationNotRequired
	err := next.ValidateTransitionTo(later)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

func validSagaInstance(now time.Time) SagaInstance {
	return SagaInstance{
		ID:                SagaID(uuid.MustParse("11111111-1111-1111-1111-111111111111")),
		Type:              SagaType("checkout"),
		DefinitionVersion: SagaDefinitionVersion(1),
		BusinessKey:       SagaBusinessKey("checkout:order-123"),
		Phase:             ForwardSagaPhase,
		Status:            RunningSagaStatus,
		CurrentStep:       SagaStep("create_payment_session"),
		StepAttemptCount:  0,
		Version:           0,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func validSagaForwardStepCompletion(now time.Time) SagaForwardStepCompletion {
	return SagaForwardStepCompletion{
		ID:                SagaStepCompletionID(uuid.MustParse("22222222-2222-2222-2222-222222222222")),
		SagaID:            SagaID(uuid.MustParse("11111111-1111-1111-1111-111111111111")),
		Step:              SagaStep("reserve_resource"),
		Sequence:          1,
		AttemptCount:      1,
		CompensationState: SagaStepNotCompensated,
		CompletedAt:       now,
	}
}
