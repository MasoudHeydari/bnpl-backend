package config

import (
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

// Usecases groups configuration sections used to build application-layer
// use cases.
//
// This struct is part of the adapter/config layer: it translates YAML
// configuration into constructor inputs, but it must not contain business
// rules itself.
type Usecases struct {
	Migrations Migrations `yaml:"migration"`
}

// Migrations contains filesystem-backed migration settings.
//
// filesDir points to the directory that contains ordered SQL migration files
// used by `bnpl db migrate`. The path is read from:
//
//	usecases:
//	  migration:
//	    files-dir: ...
//
// The field is intentionally unexported because callers should construct the
// migration use case through the config package instead of reading raw config.
type Migrations struct {
	filesDir string `yaml:"files-dir"`
}

// newUseCase constructs a migration use case from the configured migration
// directory, database pool, and migration repository.
//
// The caller retains ownership of p and r. The returned use case uses filesDir
// only as a filesystem path and does not validate the directory until migration
// execution.
func (m Migrations) newUseCase(
	p repo.Pool, r repo.Migrations,
) *migrationsuc.UseCase {
	return migrationsuc.New(p, r, m.filesDir)
}
