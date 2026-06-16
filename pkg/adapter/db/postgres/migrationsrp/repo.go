// Package migrationsrp implements the migration repository with PostgreSQL SQL
// execution primitives. It belongs to the infrastructure layer and provides
// the concrete repository used by the migration application use case.
package migrationsrp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Repo is the PostgreSQL implementation of the core migration repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL migration repository.
func New() *Repo {
	return &Repo{}
}

// NewInitializer binds a schema initializer to tx using the embedded SQL
// assets shipped with this package.
func NewInitializer(tx repo.Tx) repo.SchemaInitializer {
	return NewSettler(tx)
}

type connQueryer struct {
	*postgres.Conn
}

// Conn binds the repository to a non-transactional connection scope.
func (cars *Repo) Conn(c repo.Conn) repo.MigrationsConnQueryer {
	cc := c.(*postgres.Conn)
	return connQueryer{Conn: cc}
}

func (cq connQueryer) Migrate(ctx context.Context, migName, migStatements string) error {
	return Migrate(ctx, cq.Conn, migName, migStatements)
}

type txQueryer struct {
	*postgres.Tx
}

// Tx binds the repository to a transaction scope so execution and migration
// registration can commit atomically.
func (cars *Repo) Tx(tx repo.Tx) repo.MigrationsTxQueryer {
	tt := tx.(*postgres.Tx)
	return txQueryer{Tx: tt}
}

func (tq txQueryer) Migrate(ctx context.Context, migName, migStatements string) error {
	return Migrate(ctx, tq.Tx, migName, migStatements)
}

func (tq txQueryer) Register(ctx context.Context, migName string) error {
	return Register(ctx, tq.Tx, migName)
}
