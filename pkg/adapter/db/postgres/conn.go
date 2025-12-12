package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

var emptyTxOptions pgx.TxOptions

type Conn struct {
	*pgxpool.Conn
}

type TxHandler = repo.TxHandler

func (c *Conn) Tx(ctx context.Context, f TxHandler) (err error) {
	var tx pgx.Tx
	tx, err = c.Conn.BeginTx(ctx, emptyTxOptions)
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

func (c *Conn) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	result, err := c.Conn.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (c *Conn) Query(ctx context.Context, sql string, args ...any) (repo.Rows, error) {
	rows, err := c.Conn.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	return rowsAdapter{Rows: rows}, nil
}

func (c *Conn) IsConn() {}
