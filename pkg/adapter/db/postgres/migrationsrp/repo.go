package migrationsrp

import (
	"context"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type Repo struct {
}

func New() *Repo {
	return &Repo{}
}

func NewInitializer(tx repo.Tx) repo.SchemaInitializer {
	return NewSettler(tx)
}

type connQueryer struct {
	*postgres.Conn
}

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
