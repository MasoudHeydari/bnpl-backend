package inprocess_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	errorreportingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting/noop"
	taskinprocess "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/inprocess"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

func mustNewRuntime(t *testing.T, cfg taskinprocess.Config, logger *mock.Logger) *taskinprocess.Runtime {
	t.Helper()

	runtime, err := taskinprocess.New(cfg, logger, errorreportingnoop.New())
	require.NoError(t, err)

	return runtime
}

func TestRegister_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		task    task.Task
		wantErr error
	}{
		{
			name: "missing name",
			task: task.Task{
				Enabled: true,
				Handler: "handler",
				Trigger: task.Trigger{Type: task.TriggerManual},
				Job:     func(context.Context) error { return nil },
			},
			wantErr: taskinprocess.ErrTaskNameRequired,
		},
		{
			name: "missing handler",
			task: task.Task{
				Enabled: true,
				Name:    "task",
				Trigger: task.Trigger{Type: task.TriggerManual},
				Job:     func(context.Context) error { return nil },
			},
			wantErr: taskinprocess.ErrTaskHandlerRequired,
		},
		{
			name: "missing job",
			task: task.Task{
				Enabled: true,
				Name:    "task",
				Handler: "handler",
				Trigger: task.Trigger{Type: task.TriggerManual},
			},
			wantErr: taskinprocess.ErrTaskJobRequired,
		},
		{
			name: "unsupported trigger",
			task: task.Task{
				Enabled: true,
				Name:    "task",
				Handler: "handler",
				Trigger: task.Trigger{Type: "unknown"},
				Job:     func(context.Context) error { return nil },
			},
			wantErr: taskinprocess.ErrUnsupportedTrigger,
		},
		{
			name: "cron without expression",
			task: task.Task{
				Enabled: true,
				Name:    "task",
				Handler: "handler",
				Trigger: task.Trigger{Type: task.TriggerCron},
				Job:     func(context.Context) error { return nil },
			},
			wantErr: taskinprocess.ErrInvalidTrigger,
		},
		{
			name: "interval without positive duration",
			task: task.Task{
				Enabled: true,
				Name:    "task",
				Handler: "handler",
				Trigger: task.Trigger{Type: task.TriggerInterval},
				Job:     func(context.Context) error { return nil },
			},
			wantErr: taskinprocess.ErrInvalidTrigger,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := mock.NewLogger(t)
			runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

			err := runtime.Register(tc.task)

			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.wantErr))
		})
	}
}

func TestRegister_DisabledTaskIsSkipped(t *testing.T) {
	logger := mock.NewLogger(t)

	expectWith(logger, "task_name", "disabled_task", "task_handler", "disabled_handler").
		Return(logger).
		Once()
	logger.EXPECT().Info("task registration skipped because task is disabled").Once()

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: false,
		Name:    "disabled_task",
		Handler: "disabled_handler",
		Trigger: task.Trigger{Type: task.TriggerInterval, Interval: time.Second},
		Job:     func(context.Context) error { return nil },
	})

	require.NoError(t, err)
}

func TestDispatch_ValidationErrors(t *testing.T) {
	logger := mock.NewLogger(t)
	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Dispatch(context.Background(), task.Command{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, taskinprocess.ErrCommandHandlerRequired))

	err = runtime.Dispatch(context.Background(), task.Command{
		Handler: "unknown_handler",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, taskinprocess.ErrUnknownHandler))
}

func TestDispatch_RegisteredHandlerExecutesJob(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	var runs atomic.Int64

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "manual_task",
		Handler: "manual_handler",
		Trigger: task.Trigger{Type: task.TriggerManual},
		Job: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	err = runtime.Dispatch(context.Background(), task.Command{
		ID:      "command-1",
		Handler: "manual_handler",
		Payload: map[string]any{
			"id": "123",
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return runs.Load() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestDispatch_ConcurrentDispatchesDoNotRace(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	const dispatchCount = 50

	var runs atomic.Int64

	runtime := mustNewRuntime(t, taskinprocess.Config{
		MaxConcurrentRuns: dispatchCount,
	}, logger)

	for i := 0; i < dispatchCount; i++ {
		err := runtime.Register(task.Task{
			Enabled: true,
			Name:    fmt.Sprintf("concurrent_task_%d", i),
			Handler: fmt.Sprintf("concurrent_handler_%d", i),
			Trigger: task.Trigger{Type: task.TriggerManual},
			Job: func(context.Context) error {
				runs.Add(1)
				return nil
			},
		})
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	wg.Add(dispatchCount)

	for i := 0; i < dispatchCount; i++ {
		go func(commandID int) {
			defer wg.Done()

			dispatchErr := runtime.Dispatch(context.Background(), task.Command{
				ID:      fmt.Sprintf("command-%d", commandID),
				Handler: fmt.Sprintf("concurrent_handler_%d", commandID),
			})
			require.NoError(t, dispatchErr)
		}(i)
	}

	wg.Wait()

	require.Eventually(t, func() bool {
		return runs.Load() == dispatchCount
	}, time.Second, 10*time.Millisecond)
}

func TestDispatch_SameHandlerRejectsOverlap(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)
	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "exclusive_task",
		Handler: "exclusive_handler",
		Trigger: task.Trigger{Type: task.TriggerManual},
		Job: func(context.Context) error {
			close(started)
			<-release
			close(done)
			return nil
		},
	})
	require.NoError(t, err)

	require.NoError(t, runtime.Dispatch(context.Background(), task.Command{
		ID:      "command-1",
		Handler: "exclusive_handler",
	}))
	<-started

	overlapErr := runtime.Dispatch(context.Background(), task.Command{
		ID:      "command-2",
		Handler: "exclusive_handler",
	})
	require.ErrorIs(t, overlapErr, taskinprocess.ErrRuntimeBusy)

	close(release)
	<-done
}

func TestStart_IntervalRunOnStartExecutesJob(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	var runs atomic.Int64

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "interval_task",
		Handler: "interval_handler",
		Trigger: task.Trigger{
			Type:       task.TriggerInterval,
			Interval:   time.Hour,
			RunOnStart: true,
		},
		Job: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- runtime.Start(ctx)
	}()

	require.Eventually(t, func() bool {
		return runs.Load() == 1
	}, time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after context cancellation")
	}
}

func TestStart_IntervalDoesNotRunImmediatelyWhenRunOnStartFalse(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	var runs atomic.Int64

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "interval_task",
		Handler: "interval_handler",
		Trigger: task.Trigger{
			Type:       task.TriggerInterval,
			Interval:   time.Hour,
			RunOnStart: false,
		},
		Job: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- runtime.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(0), runs.Load())

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after context cancellation")
	}
}

func TestStart_IntervalRepeats(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	var runs atomic.Int64

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "interval_task",
		Handler: "interval_handler",
		Trigger: task.Trigger{
			Type:     task.TriggerInterval,
			Interval: 10 * time.Millisecond,
		},
		Job: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- runtime.Start(ctx)
	}()

	require.Eventually(t, func() bool {
		return runs.Load() >= 2
	}, time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after context cancellation")
	}
}

func TestStart_ManualTaskDoesNotExecuteAutomatically(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	var runs atomic.Int64

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "manual_task",
		Handler: "manual_handler",
		Trigger: task.Trigger{Type: task.TriggerManual},
		Job: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- runtime.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(0), runs.Load())

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after context cancellation")
	}
}

func TestStart_CronInvalidTimezoneReturnsError(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	runtime := mustNewRuntime(t, taskinprocess.Config{
		DefaultTimezone: "Invalid/Timezone",
	}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "cron_task",
		Handler: "cron_handler",
		Trigger: task.Trigger{
			Type: task.TriggerCron,
			Cron: "0 2 * * *",
		},
		Job: func(context.Context) error { return nil },
	})
	require.NoError(t, err)

	err = runtime.Start(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, taskinprocess.ErrInvalidTrigger))
}

func TestStart_CronInvalidExpressionReturnsError(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "cron_task",
		Handler: "cron_handler",
		Trigger: task.Trigger{
			Type: task.TriggerCron,
			Cron: "invalid cron",
		},
		Job: func(context.Context) error { return nil },
	})
	require.NoError(t, err)

	err = runtime.Start(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, taskinprocess.ErrInvalidTrigger))
}

func TestStart_CronTaskStartsAndStops(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	runtime := mustNewRuntime(t, taskinprocess.Config{
		DefaultTimezone: "Asia/Tehran",
	}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "cron_task",
		Handler: "cron_handler",
		Trigger: task.Trigger{
			Type: task.TriggerCron,
			Cron: "0 2 * * *",
		},
		Job: func(context.Context) error { return nil },
	})
	require.NoError(t, err)

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

func TestExecute_JobErrorDoesNotStopRuntime(t *testing.T) {
	logger := mock.NewLogger(t)
	allowLifecycleLogs(logger)

	var runs atomic.Int64

	runtime := mustNewRuntime(t, taskinprocess.Config{}, logger)

	err := runtime.Register(task.Task{
		Enabled: true,
		Name:    "failing_interval_task",
		Handler: "failing_interval_handler",
		Trigger: task.Trigger{
			Type:       task.TriggerInterval,
			Interval:   time.Hour,
			RunOnStart: true,
		},
		Job: func(context.Context) error {
			runs.Add(1)
			return errors.New("boom")
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- runtime.Start(ctx)
	}()

	require.Eventually(t, func() bool {
		return runs.Load() == 1
	}, time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after context cancellation")
	}
}

func allowLifecycleLogs(logger *mock.Logger) {
	logger.EXPECT().With(testifymock.Anything).Return(logger).Maybe()
	logger.EXPECT().Info(testifymock.Anything).Maybe()
	logger.EXPECT().Error(testifymock.Anything).Maybe()
}

func expectWith(logger *mock.Logger, fields ...any) *mock.Logger_With_Call {
	return logger.EXPECT().With(testifymock.MatchedBy(func(got []any) bool {
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
