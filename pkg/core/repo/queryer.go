package repo

import "context"

// Queryer abstracts direct SQL execution and row queries.
// The interface is intentionally small so both connections and transactions can
// satisfy it.
type Queryer interface {
	// Exec executes a statement and returns the number of affected rows when the
	// underlying driver can report it.
	Exec(ctx context.Context, sql string, args ...any) (count int64, err error)

	// Query executes a query and returns a row iterator.
	// The caller must close the returned Rows.
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
}
