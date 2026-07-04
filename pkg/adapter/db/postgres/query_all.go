package postgres

import (
	"context"
	"fmt"
)

// QueryAll executes query and scans all rows into a slice of T.
func QueryAll[T any, Q Queryer](
	ctx context.Context,
	q Q,
	operation string,
	query string,
	args ...any,
) ([]T, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, MapError(err))
	}

	defer rows.Close()

	values, err := ValuesInto[T](MustRows(rows))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}

	return values, nil
}
