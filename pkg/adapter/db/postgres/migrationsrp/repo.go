package migrationsrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.Migrations = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the migrations repository.
type Repo struct{}

// New returns a stateless PostgreSQL migrations repository.
func New() *Repo { return &Repo{} }

// NewInitializer returns a schema initializer bound to the given transaction.
func NewInitializer(tx repo.Tx) repo.SchemaInitializer { return NewSettler(tx) }

type connQueryer struct{ *postgres.Conn }

var _ repo.MigrationsConnQueryer = (*connQueryer)(nil)

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.MigrationsConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

func (q connQueryer) AcquireLock(ctx context.Context) error { return AcquireLock(ctx, q.Conn) }
func (q connQueryer) ReleaseLock(ctx context.Context) error { return ReleaseLock(ctx, q.Conn) }
func (q connQueryer) ListApplied(ctx context.Context) ([]model.Migration, error) {
	return ListApplied(ctx, q.Conn)
}
func (q connQueryer) Migrate(ctx context.Context, name, statements string) error {
	return Migrate(ctx, q.Conn, name, statements)
}
func (q connQueryer) Register(ctx context.Context, name, checksum string, at time.Time) (model.Migration, error) {
	return Register(ctx, q.Conn, name, checksum, at)
}
func (q connQueryer) MarkDirty(ctx context.Context, name, checksum string, at time.Time) (model.Migration, error) {
	return MarkDirty(ctx, q.Conn, name, checksum, at)
}
func (q connQueryer) MarkApplied(ctx context.Context, name, checksum string, at time.Time) (model.Migration, error) {
	return MarkApplied(ctx, q.Conn, name, checksum, at)
}

type txQueryer struct{ *postgres.Tx }

var _ repo.MigrationsTxQueryer = (*txQueryer)(nil)

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.MigrationsTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}
func (q txQueryer) Migrate(ctx context.Context, name, statements string) error {
	return Migrate(ctx, q.Tx, name, statements)
}
func (q txQueryer) Register(ctx context.Context, name, checksum string, at time.Time) (model.Migration, error) {
	return Register(ctx, q.Tx, name, checksum, at)
}
