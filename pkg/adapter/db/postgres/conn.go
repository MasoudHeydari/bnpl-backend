package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

var emptyTxOptions pgx.TxOptions

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
func (c *Conn) Tx(ctx context.Context, f TxHandler) (err error) {
	var tx pgx.Tx
	tx, err = c.BeginTx(ctx, emptyTxOptions)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			err = tx.Rollback(ctx)
			if err == nil {
				err = fmt.Errorf("panicked: %v", err)
				return
			}
			err = fmt.Errorf("panicked: %v, rollback: %w", r, err)
			return
		}
		if err != nil {
			if err2 := tx.Rollback(ctx); err2 != nil {
				err = fmt.Errorf("handler: %w, rollback: %w", err, err2)
				return
			}
			err = fmt.Errorf("handler: %w", err)
			return
		}
		err = tx.Commit(ctx)
		if err != nil {
			err = fmt.Errorf("commit: %w", err)
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
	rows, err := c.Conn.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	return rowsAdapter{Rows: rows}, nil
}

// IsConn marks the adapter as a repository connection implementation.
func (c *Conn) IsConn() {}
