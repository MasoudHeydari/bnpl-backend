package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5"
)

type rowsAdapter struct {
	pgx.Rows
}

func (ra rowsAdapter) Values() ([]any, error) {
	var err error
	names := ra.FieldDescriptions()
	if err = ra.Err(); err != nil {
		return nil, fmt.Errorf("column-names: %w", err)
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

func (ra rowsAdapter) Close() {
	// returned error maybe checked by calling the Err() method.
	ra.Rows.Close()
}
