package bootstrap

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cache"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Infrastructure holds the runtime adapters assembled for the application.
type Infrastructure struct {
	Trace      trace.Provider
	Clock      clock.Clock
	Logger     log.Logger
	Cache      cache.Cache
	Publisher  messaging.Publisher
	Subscriber messaging.Subscriber
	Task       task.Runtime
}

// NewInfrastructures constructs the infrastructure adapters required by App.
func (a *App) NewInfrastructures() (Infrastructure, error) {
	traceProvider, err := a.Config.NewTraceInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize trace: %w", err)
	}

	// Single system clock instance shared by all use cases that need time.
	systemClock, err := a.Config.NewSystemClockInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize system clock: %w", err)
	}

	logger, err := a.Config.NewLoggerInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize logger: %w", err)
	}

	cacheInfra, err := a.Config.NewCacheInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize cache: %w", err)
	}

	publisher, err := a.Config.NewMessagingPublisherInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize messaging publisher: %w", err)
	}

	subscriber, err := a.Config.NewMessagingSubscriberInfrastructure(logger)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize messaging subscriber: %w", err)
	}

	taskInfra, err := a.Config.NewTaskInfrastructure(logger)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize task: %w", err)
	}

	return Infrastructure{
		Trace:      traceProvider,
		Clock:      systemClock,
		Logger:     logger,
		Cache:      cacheInfra,
		Publisher:  publisher,
		Subscriber: subscriber,
		Task:       taskInfra,
	}, nil
}
