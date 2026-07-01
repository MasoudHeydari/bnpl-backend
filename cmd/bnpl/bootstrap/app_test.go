package bootstrap

import (
	"errors"
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testRegisterTasksApp(t *testing.T, cfg *config.Config, runtime *mock.Runtime) *App {
	t.Helper()

	app := testAppWithInfrastructure(t, cfg)
	app.Infrastructure.Task = runtime

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	return app
}

func TestRegisterTasks_DisabledTasksSkipped(t *testing.T) {
	t.Parallel()

	runtime := mock.NewRuntime(t)
	app := testRegisterTasksApp(t, noopTestConfig(), runtime)

	err := app.RegisterTasks()
	require.NoError(t, err)
}

func TestRegisterTasks_EnabledTaskRegisters(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Infrastructure.Task.Scheduled.OutboxPublisher = config.TaskDefinition{
		Enabled: true,
		Handler: "outbox_publisher",
		Trigger: config.TaskTrigger{
			Type:     task.TriggerInterval,
			Interval: "5s",
		},
	}

	runtime := mock.NewRuntime(t)
	runtime.EXPECT().Register(testifymock.MatchedBy(func(registered task.Task) bool {
		return registered.Name == "outbox_publisher" &&
			registered.Handler == "outbox_publisher" &&
			registered.Enabled &&
			registered.Trigger.Type == task.TriggerInterval &&
			registered.Trigger.Interval == 5*time.Second &&
			registered.Job != nil
	})).Return(nil).Once()

	app := testRegisterTasksApp(t, cfg, runtime)

	err := app.RegisterTasks()
	require.NoError(t, err)
}

func TestRegisterTasks_InvalidTriggerReturnsError(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Infrastructure.Task.Scheduled.OutboxPublisher = config.TaskDefinition{
		Enabled: true,
		Handler: "outbox_publisher",
		Trigger: config.TaskTrigger{
			Type:     task.TriggerInterval,
			Interval: "not-a-duration",
		},
	}

	runtime := mock.NewRuntime(t)
	app := testRegisterTasksApp(t, cfg, runtime)

	err := app.RegisterTasks()
	require.Error(t, err)
	require.ErrorContains(t, err, "build trigger for task")
}

func TestRegisterTasks_UnknownHandlerReturnsError(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Infrastructure.Task.Scheduled.OutboxPublisher = config.TaskDefinition{
		Enabled: true,
		Handler: "unknown_handler",
		Trigger: config.TaskTrigger{
			Type: task.TriggerManual,
		},
	}

	runtime := mock.NewRuntime(t)
	app := testRegisterTasksApp(t, cfg, runtime)

	err := app.RegisterTasks()
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve job for task")
}

func TestRegisterTasks_RegisterFailureReturnsError(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Infrastructure.Task.Scheduled.OutboxPublisher = config.TaskDefinition{
		Enabled: true,
		Handler: "outbox_publisher",
		Trigger: config.TaskTrigger{
			Type:     task.TriggerInterval,
			Interval: "5s",
		},
	}

	registerErr := errors.New("register failed")
	runtime := mock.NewRuntime(t)
	runtime.EXPECT().Register(testifymock.Anything).Return(registerErr).Once()

	app := testRegisterTasksApp(t, cfg, runtime)

	err := app.RegisterTasks()
	require.Error(t, err)
	require.ErrorIs(t, err, registerErr)
	require.ErrorContains(t, err, "register task")
}
