package config

import (
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/usecase/migrationsuc"
)

// Usecases groups configuration sections that feed application-layer use case
// constructors.
type Usecases struct {
	Migrations Migrations
}

// Migrations holds migration use case settings.
// migDirPath is not populated from YAML today because the field lacks a yaml tag;
// db migrate therefore receives an empty directory until wiring is completed.
type Migrations struct {
	migDirPath string ``
}

// NewUseCase constructs the migration use case with the configured migration
// directory path, connection pool, and migration repository implementation.
func (m Migrations) NewUseCase(
	p repo.Pool, r repo.Migrations,
) *migrationsuc.UseCase {
	return migrationsuc.New(p, r, m.migDirPath)
}
