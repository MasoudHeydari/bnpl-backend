package bootstrap

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/outboxrp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Repos holds repository adapters assembled for the application.
type Repos struct {
	Migrations repo.Migrations
	Outbox     repo.Outbox
}

// NewRepos constructs the repository adapter set used by App.
func NewRepos() Repos {
	return Repos{
		Migrations: migrationsrp.New(),
		Outbox:     outboxrp.New(),
	}
}
