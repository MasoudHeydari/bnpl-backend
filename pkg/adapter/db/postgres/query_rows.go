package postgres

import (
	"context"
	"fmt"
)

// QueryRows executes sql and returns the concrete PostgreSQL Rows adapter.
//
// Prefer this over Query + MustRows inside postgres adapters: it avoids a
// round-trip through repo.Rows and the associated type assertion panic.
func QueryRows[Q Queryer](
	ctx context.Context,
	q Q,
	sql string,
	args ...any,
) (Rows, error) {
	const op = "postgres.QueryRows"

	switch c := any(q).(type) {
	case *Conn:
		rows, err := c.Conn.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}

		return rowsAdapter{Rows: rows}, nil

	case *Tx:
		rows, err := c.Tx.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}

		return rowsAdapter{Rows: rows}, nil

	default:
		return nil, fmt.Errorf("%s: unsupported queryer %T", op, q)
	}
}
