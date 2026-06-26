package gobreaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/gobreaker"
	corecb "github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
)

func TestCircuitBreaker_Execute_runsFunctionWhenClosed(t *testing.T) {
	t.Parallel()

	cb := gobreaker.New(gobreaker.Config{
		MaxRequests: 1,
		Interval:    time.Minute,
		Timeout:     time.Minute,
	})

	called := false

	err := cb.Execute(context.Background(), "dependency", func(ctx context.Context) error {
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

	cb := gobreaker.New(gobreaker.Config{
		MaxRequests: 1,
		Interval:    time.Minute,
		Timeout:     time.Minute,
	})

	expectedErr := errors.New("dependency failed")

	err := cb.Execute(context.Background(), "dependency", func(ctx context.Context) error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestCircuitBreaker_Execute_returnsErrOpenWhenCircuitIsOpen(t *testing.T) {
	t.Parallel()

	cb := gobreaker.New(gobreaker.Config{
		MaxRequests: 1,
		Interval:    time.Minute,
		Timeout:     time.Minute,
	})

	expectedErr := errors.New("dependency failed")

	err := cb.Execute(context.Background(), "dependency", func(ctx context.Context) error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected first call to return dependency error, got %v", err)
	}

	called := false

	err = cb.Execute(context.Background(), "dependency", func(ctx context.Context) error {
		called = true
		return nil
	})

	if !errors.Is(err, corecb.ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}

	if called {
		t.Fatal("expected function not to be called while circuit is open")
	}
}

func TestCircuitBreaker_Execute_keepsIndependentStatePerName(t *testing.T) {
	t.Parallel()

	cb := gobreaker.New(gobreaker.Config{
		MaxRequests: 1,
		Interval:    time.Minute,
		Timeout:     time.Minute,
	})

	expectedErr := errors.New("dependency-a failed")

	err := cb.Execute(context.Background(), "dependency-a", func(ctx context.Context) error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected dependency-a failure, got %v", err)
	}

	called := false

	err = cb.Execute(context.Background(), "dependency-b", func(ctx context.Context) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("expected dependency-b to remain closed, got %v", err)
	}

	if !called {
		t.Fatal("expected dependency-b function to be called")
	}
}
