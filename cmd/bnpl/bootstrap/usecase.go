package bootstrap

import (
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
)

// UseCases holds the application use cases assembled for App.
type UseCases struct {
	Migrations      *migrationsuc.UseCase
	OutboxPublisher *outboxpublisheruc.UseCase
}

// NewUseCases constructs the use cases required by App.
func (a *App) NewUseCases() (UseCases, error) {
	migrationsUseCase := a.Config.NewMigrationsUseCase(
		a.Pool,
		a.Repos.Migrations,
	)

	outboxPublisherUseCase := a.Config.NewOutboxPublisherUseCase(
		a.Pool,
		a.Repos.Outbox,
		a.Infrastructure.Publisher,
		a.Infrastructure.Clock,
		a.Infrastructure.Logger,
	)

	return UseCases{
		Migrations:      migrationsUseCase,
		OutboxPublisher: outboxPublisherUseCase,
	}, nil
}
