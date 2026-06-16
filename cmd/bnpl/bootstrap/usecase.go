package bootstrap

import (
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

// UseCases holds the application use cases assembled for App.
type UseCases struct {
	Migrations *migrationsuc.UseCase
}

// NewUseCases constructs the use cases required by App.
func (a *App) NewUseCases() (UseCases, error) {
	migrationsUseCase := a.Config.NewMigrationsUseCase(
		a.Pool,
		a.Repos.Migrations,
	)

	return UseCases{
		Migrations: migrationsUseCase,
	}, nil
}
