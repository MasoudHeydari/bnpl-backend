package postgres

import (
	"context"
	"fmt"
)

// QueryOne executes query and scans the first row into T.
func QueryOne[T any, Q Queryer](
	ctx context.Context,
	q Q,
	operation string,
	query string,
	args ...any,
) (T, error) {
	var zero T

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", operation, MapError(err))
	}
	defer rows.Close()

	value, err := ValueInto[T](rows.(Rows))
	if err != nil {
		return zero, fmt.Errorf("%s: %w", operation, err)
	}

	return value, nil
}
