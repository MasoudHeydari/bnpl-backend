package composite_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/composite"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

type recordingRuntime struct {
	mu         sync.Mutex
	registered []task.Task
	dispatched []task.Command
}

func (r *recordingRuntime) Register(t task.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered = append(r.registered, t)
	return nil
}

func (r *recordingRuntime) Dispatch(_ context.Context, command task.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dispatched = append(r.dispatched, command)
	return nil
}

func (r *recordingRuntime) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (r *recordingRuntime) Close() error {
	return nil
}

func TestNewRequiresBothFixedBackends(t *testing.T) {
	t.Parallel()

	temporal := &recordingRuntime{}
	_, err := composite.New(nil, temporal)
	require.ErrorIs(t, err, composite.ErrInProcessRuntimeRequired)

	inProcess := &recordingRuntime{}
	_, err = composite.New(inProcess, nil)
	require.ErrorIs(t, err, composite.ErrTemporalRuntimeRequired)
}

func TestRegisterRoutesEnabledTaskByExecutionMode(t *testing.T) {
	t.Parallel()

	inProcess := &recordingRuntime{}
	temporal := &recordingRuntime{}
	runtime, err := composite.New(inProcess, temporal)
	require.NoError(t, err)

	localTask := task.Task{
		Name:          "local",
		Enabled:       true,
		ExecutionMode: task.ExecutionModeInProcess,
		Handler:       "local_handler",
	}
	durableTask := task.Task{
		Name:          "durable",
		Enabled:       true,
		ExecutionMode: task.ExecutionModeTemporal,
		Handler:       "durable_handler",
	}

	require.NoError(t, runtime.Register(localTask))
	require.NoError(t, runtime.Register(durableTask))

	require.Len(t, inProcess.registered, 2)
	require.Equal(t, "local", inProcess.registered[0].Name)
	require.True(t, inProcess.registered[0].Enabled)
	require.Equal(t, "durable", inProcess.registered[1].Name)
	require.False(t, inProcess.registered[1].Enabled)

	require.Len(t, temporal.registered, 2)
	require.Equal(t, "local", temporal.registered[0].Name)
	require.False(t, temporal.registered[0].Enabled)
	require.Nil(t, temporal.registered[0].Job)
	require.Equal(t, "durable", temporal.registered[1].Name)
	require.True(t, temporal.registered[1].Enabled)
}

func TestRegisterDisabledTaskReachesBothBackends(t *testing.T) {
	t.Parallel()

	inProcess := &recordingRuntime{}
	temporal := &recordingRuntime{}
	runtime, err := composite.New(inProcess, temporal)
	require.NoError(t, err)

	require.NoError(t, runtime.Register(task.Task{
		Name:    "disabled",
		Enabled: false,
		Handler: "disabled_handler",
	}))

	require.Len(t, inProcess.registered, 1)
	require.Len(t, temporal.registered, 1)
}

func TestDispatchUsesRegistrationRoute(t *testing.T) {
	t.Parallel()

	inProcess := &recordingRuntime{}
	temporal := &recordingRuntime{}
	runtime, err := composite.New(inProcess, temporal)
	require.NoError(t, err)

	require.NoError(t, runtime.Register(task.Task{
		Name:          "durable",
		Enabled:       true,
		ExecutionMode: task.ExecutionModeTemporal,
		Handler:       "durable_handler",
	}))

	command := task.Command{ID: "command-1", Handler: "durable_handler"}
	require.NoError(t, runtime.Dispatch(context.Background(), command))

	require.Empty(t, inProcess.dispatched)
	require.Equal(t, []task.Command{command}, temporal.dispatched)
}

func TestRegisterEnabledTaskRequiresSupportedExecutionMode(t *testing.T) {
	t.Parallel()

	runtime, err := composite.New(&recordingRuntime{}, &recordingRuntime{})
	require.NoError(t, err)

	err = runtime.Register(task.Task{Name: "missing", Enabled: true})
	require.ErrorIs(t, err, composite.ErrExecutionModeRequired)

	err = runtime.Register(task.Task{
		Name:          "unsupported",
		Enabled:       true,
		ExecutionMode: task.ExecutionMode("other"),
	})
	require.ErrorIs(t, err, composite.ErrUnsupportedExecutionMode)
}
