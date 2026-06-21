package noop_test

import (
	"context"
	"testing"
	"time"

	tasknoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/noop"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	logger := mock.NewLogger(t)

	expectWith(
		logger,
		"task_name", "test_task",
		"task_handler", "test_handler",
	).Return(logger).Once()

	logger.
		EXPECT().
		Info("task registration ignored by noop runtime").
		Once()

	runtime := tasknoop.New(logger)

	err := runtime.Register(task.Task{
		Name:    "test_task",
		Handler: "test_handler",
	})

	require.NoError(t, err)
}

func TestDispatch(t *testing.T) {
	logger := mock.NewLogger(t)

	expectWith(
		logger,
		"command_id", "command-1",
		"task_handler", "test_handler",
	).Return(logger).Once()

	logger.
		EXPECT().
		Info("task dispatch ignored by noop runtime").
		Once()

	runtime := tasknoop.New(logger)

	err := runtime.Dispatch(context.Background(), task.Command{
		ID:      "command-1",
		Handler: "test_handler",
		Payload: map[string]any{
			"id": "123",
		},
	})

	require.NoError(t, err)
}

func TestStartReturnsOnContextCancellation(t *testing.T) {
	logger := mock.NewLogger(t)

	logger.
		EXPECT().
		Info("starting noop task runtime").
		Once()

	logger.
		EXPECT().
		Info("stopping noop task runtime").
		Once()

	runtime := tasknoop.New(logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- runtime.Start(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after context cancellation")
	}
}

func expectWith(
	logger *mock.Logger,
	fields ...any,
) *mock.Logger_With_Call {
	return logger.
		EXPECT().
		With(testifymock.MatchedBy(func(got []any) bool {
			if len(got) != len(fields) {
				return false
			}

			for i := range fields {
				if got[i] != fields[i] {
					return false
				}
			}

			return true
		}))
}
