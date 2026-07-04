package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var emptyTxOptions pgx.TxOptions

// Verify Conn implements the core repository connection contract.
var _ repo.Conn = (*Conn)(nil)

// Conn adapts a pooled pgx connection to the core repository Conn interface.
// A Conn is leased from Pool.Conn and should not be shared outside the
// callback that received it.
type Conn struct {
	*pgxpool.Conn
}

// TxHandler aliases the core transaction callback type for local adapter use.
type TxHandler = repo.TxHandler

// Tx executes f within a database transaction.
// It commits when f returns nil, rolls back when f returns an error or panics,
// and wraps rollback or commit failures into the returned error.
func (c *Conn) Tx(ctx context.Context, f TxHandler) error {
	return c.withTx(ctx, emptyTxOptions, f)
}

// ReadSnapshot holds a coherent committed view across multi-query account reads.
func (c *Conn) ReadSnapshot(ctx context.Context, f TxHandler) error {
	return c.withTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, f)
}

func (c *Conn) withTx(ctx context.Context, options pgx.TxOptions, f TxHandler) (err error) {
	var tx pgx.Tx
	const op = "postgres.Conn.Tx"

	tx, err = c.BeginTx(ctx, options)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}

	defer func() {
		if r := recover(); r != nil {
			err = tx.Rollback(ctx)
			if err == nil {
				err = fmt.Errorf("%s: panicked: %v", op, r)
				return
			}

			err = fmt.Errorf("%s: panicked: %v, rollback: %w", op, r, err)
			return
		}

		if err != nil {
			if err2 := tx.Rollback(ctx); err2 != nil {
				err = fmt.Errorf("%s: handler: %w, rollback: %w", op, err, err2)
				return
			}

			err = fmt.Errorf("%s: handler: %w", op, err)
			return
		}

		err = tx.Commit(ctx)
		if err != nil {
			err = fmt.Errorf("%s: commit: %w", op, err)
		}
	}()

	tt := &Tx{Tx: tx}
	return f(ctx, tt)
}

// Exec executes sql on the underlying connection and returns affected rows.
func (c *Conn) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	result, err := c.Conn.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// Query executes sql on the underlying connection and returns adapted rows.
// Callers must close the returned iterator.
func (c *Conn) Query(ctx context.Context, sql string, args ...any) (repo.Rows, error) {
	rows, err := c.Conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	return rowsAdapter{Rows: rows}, nil
}

// IsConn marks the adapter as a repository connection implementation.
func (c *Conn) IsConn() {}
