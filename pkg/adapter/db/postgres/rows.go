package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ErrNoRows is re-exported for callers that need direct access to the
// underlying pgx sentinel error.
//
// New code should generally prefer MapError and cerr-based error handling.
var ErrNoRows = pgx.ErrNoRows

// Rows defines the row iterator contract used by PostgreSQL adapters.
//
// The interface intentionally mirrors pgx.Rows to avoid leaking pgx directly
// into higher-level repository code.
type Rows interface {
	pgx.Rows
}

// rowsAdapter adapts pgx.Rows to additional helper methods used by the
// PostgreSQL infrastructure layer.
type rowsAdapter struct {
	pgx.Rows
}

// Verify rowsAdapter implements the core repository rows contract.
var _ repo.Rows = (*rowsAdapter)(nil)

// Values returns the values of the current row as a slice of any.
//
// The implementation dynamically allocates scan destinations based on the
// result column count and scans the current row into them.
func (ra rowsAdapter) Values() ([]any, error) {
	var err error

	names := ra.FieldDescriptions()

	if err = ra.Err(); err != nil {
		return nil, fmt.Errorf("column names: %w", err)
	}

	if len(names) == 0 {
		return []any{}, nil
	}

	vals := make([]any, len(names))
	valPtrs := make([]any, 0, len(names))

	for i := range vals {
		ptr := &vals[i]
		valPtrs = append(valPtrs, ptr)
	}

	err = ra.Scan(valPtrs...)

	return vals, err
}

// Close closes the underlying row iterator.
//
// Any deferred row-processing errors should be checked using Err().
func (ra rowsAdapter) Close() {
	ra.Rows.Close()
}

// ValueInto collects a single row into a struct using field-name mapping.
//
// Database errors are converted into application-level cerr errors through
// MapError.
func ValueInto[T any](row Rows) (t T, err error) {
	t, err = pgx.CollectOneRow(
		row,
		pgx.RowToStructByName[T],
	)
	if err != nil {
		return t, MapError(err)
	}

	return t, nil
}

// ValuesInto collects all rows into a slice of structs using field-name
// mapping.
//
// Database errors are converted into application-level cerr errors through
// MapError.
func ValuesInto[T any](rows Rows) (ts []T, err error) {
	ts, err = pgx.CollectRows(
		rows,
		pgx.RowToStructByName[T],
	)
	if err != nil {
		return ts, MapError(err)
	}

	return ts, nil
}
