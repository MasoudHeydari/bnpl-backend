package contractuc

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func TestCancellationSagaDefinitionForwardGraph(t *testing.T) {
	definition := &CancellationSagaDefinition{
		localStep:    &cancellationLocalStartedStep{},
		externalStep: &cancellationExternalRequestStep{},
		acceptedStep: &cancellationExternalAcceptedStep{},
	}

	if definition.InitialStep() != contractCancellationLocalStartedStep {
		t.Fatalf("unexpected initial step: %q", definition.InitialStep())
	}

	if !definition.CanTransition(
		model.ForwardSagaPhase,
		contractCancellationLocalStartedStep,
		contractCancellationExternalStep,
	) {
		t.Fatal("local cancellation start must transition to the external request")
	}

	if !definition.CanTransition(
		model.ForwardSagaPhase,
		contractCancellationExternalStep,
		contractCancellationExternalAcceptedStep,
	) {
		t.Fatal("accepted external request must transition to durable acceptance recording")
	}

	if !definition.IsForwardTerminalStep(contractCancellationExternalAcceptedStep) {
		t.Fatal("durable acceptance recording must be the forward terminal step")
	}

	if definition.IsForwardTerminalStep(contractCancellationExternalStep) {
		t.Fatal("external request must not be terminal before acceptance is recorded locally")
	}
}

func TestCancellationSagaDefinitionCompensation(t *testing.T) {
	definition := &CancellationSagaDefinition{
		localStep:    &cancellationLocalStartedStep{},
		externalStep: &cancellationExternalRequestStep{},
		acceptedStep: &cancellationExternalAcceptedStep{},
	}

	compensation, ok := definition.CompensationFor(contractCancellationLocalStartedStep)
	if !ok {
		t.Fatal("local cancellation start must define compensation")
	}

	if compensation.Mode != saga.CompensationModeDirect {
		t.Fatalf("unexpected local compensation mode: %q", compensation.Mode)
	}

	for _, step := range []model.SagaStep{
		contractCancellationExternalStep,
		contractCancellationExternalAcceptedStep,
	} {
		compensation, ok = definition.CompensationFor(step)
		if !ok {
			t.Fatalf("step %q must explicitly declare compensation policy", step)
		}

		if compensation.Mode != saga.CompensationModeNone {
			t.Fatalf("step %q must not declare direct compensation", step)
		}
	}
}

func TestCancellationSagaRetryDelay(t *testing.T) {
	const (
		base = 2 * time.Second
		max  = 10 * time.Second
	)

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 2 * time.Second},
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
		{attempt: 4, want: 10 * time.Second},
		{attempt: 20, want: 10 * time.Second},
	}

	for _, tt := range tests {
		if got := cancellationSagaRetryDelay(tt.attempt, base, max); got != tt.want {
			t.Fatalf("attempt %d: got %s, want %s", tt.attempt, got, tt.want)
		}
	}
}
