package noop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
)

func TestCircuitBreaker_Execute_runsFunction(t *testing.T) {
	t.Parallel()

	cb := noop.New()

	called := false

	err := cb.Execute(context.Background(), "test", func(ctx context.Context) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !called {
		t.Fatal("expected function to be called")
	}
}

func TestCircuitBreaker_Execute_returnsFunctionError(t *testing.T) {
	t.Parallel()

	cb := noop.New()

	expectedErr := errors.New("dependency failed")

	err := cb.Execute(context.Background(), "test", func(ctx context.Context) error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}
