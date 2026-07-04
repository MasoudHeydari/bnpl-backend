package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Tx implements the core repository transaction contract.
var _ repo.Tx = (*Tx)(nil)

// Tx adapts pgx.Tx to the core repository Tx interface.
// A Tx value is valid only within the Conn.Tx callback that created it.
type Tx struct {
	pgx.Tx
}

// Exec executes sql inside the transaction and returns affected rows.
func (tx *Tx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	result, err := tx.Tx.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// Query executes sql inside the transaction and returns adapted rows.
// Callers must close the returned iterator.
func (tx *Tx) Query(ctx context.Context, sql string, args ...any) (repo.Rows, error) {
	rows, err := tx.Tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	return rowsAdapter{Rows: rows}, nil
}

// IsTx marks the adapter as a repository transaction implementation.
func (tx *Tx) IsTx() {}
