package bootstrap

import (
	"fmt"

	adapterhealth "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/health"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Infrastructure holds the runtime adapters assembled for the application.
type Infrastructure struct {
	DependencyChecker corehealth.DependencyChecker
	Trace             trace.Provider
	Clock             clock.Clock
	Logger            log.Logger
	Cache             cache.Cache
	Publisher         messaging.Publisher
	Subscriber        messaging.Subscriber
	LifecycleState    corehealth.LifecycleState

	// CircuitBreaker protects calls to external or failure-prone dependencies.
	//
	// It should be used by adapters or infrastructure wrappers, not by domain
	// logic. Use cases should depend on business ports and remain unaware of the
	// concrete resilience mechanism.
	CircuitBreaker circuitbreaker.CircuitBreaker
	Task           task.Runtime
}

// NewInfrastructures constructs the infrastructure adapters required by App.
func (a *App) NewInfrastructures() (Infrastructure, error) {
	lifecycleState := adapterhealth.NewLifecycleState()

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

	messagingHealth, err := a.Config.NewMessagingHealthInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize messaging health: %w", err)
	}

	dependencyChecker, err := a.Config.NewDependencyCheckerInfrastructure(
		a.Pool,
		a.Repos.Health,
		cacheInfra,
		messagingHealth,
	)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize dependency checker: %w", err)
	}

	cbInfra, err := a.Config.NewCircuitBreakerInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize circuit breaker: %w", err)
	}

	taskInfra, err := a.Config.NewTaskInfrastructure(logger)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize task: %w", err)
	}

	return Infrastructure{
		DependencyChecker: dependencyChecker,
		Trace:             traceProvider,
		Clock:             systemClock,
		Logger:            logger,
		Cache:             cacheInfra,
		Publisher:         publisher,
		Subscriber:        subscriber,
		LifecycleState:    lifecycleState,
		CircuitBreaker:    cbInfra,
		Task:              taskInfra,
	}, nil
}
