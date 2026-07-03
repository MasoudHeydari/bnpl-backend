package bootstrap

import (
	"github.com/parstis/bnpl-backend/pkg/core/usecase/healthcheckuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
)

// UseCases holds the application use cases assembled for App.
type UseCases struct {
	Migrations      *migrationsuc.UseCase
	OutboxPublisher *outboxpublisheruc.UseCase
	Health          *healthcheckuc.UseCase
}

// NewUseCases constructs the use cases required by App.
func (a *App) NewUseCases() (UseCases, error) {
	migrationsUseCase := a.Config.NewMigrationsUseCase(
		a.Pool,
		a.Repos.Migrations,
		a.Infrastructure.Clock,
	)

	outboxPublisherUseCase := a.Config.NewOutboxPublisherUseCase(
		a.Pool,
		a.Repos.Outbox,
		a.Infrastructure.Publisher,
		a.Infrastructure.Clock,
		a.Infrastructure.Logger,
	)

	healthUseCase := a.Config.NewHealthUseCase(
		a.Infrastructure.LifecycleState,
		a.Infrastructure.DependencyChecker,
	)

	return UseCases{
		Migrations:      migrationsUseCase,
		OutboxPublisher: outboxPublisherUseCase,
		Health:          healthUseCase,
	}, nil
}
