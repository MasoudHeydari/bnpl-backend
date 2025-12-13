package config

import (
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/usecase/migrationsuc"
)

type Usecases struct {
	Migrations Migrations
}

type Migrations struct {
	migDirPath string ``
}

func (m Migrations) NewUseCase(
	p repo.Pool, r repo.Migrations,
) *migrationsuc.UseCase {
	return migrationsuc.New(p, r, m.migDirPath)
}
