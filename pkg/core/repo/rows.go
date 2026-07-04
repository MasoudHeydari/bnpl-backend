package repo

// Rows abstracts a forward-only query result iterator.
// Callers must always Close the iterator and then inspect Err to detect late
// driver failures.
type Rows interface {
	// Close releases resources associated with the iterator.
	Close()

	// Err returns the first error encountered during iteration.
	Err() error

	// Next prepares the next result row for reading.
	Next() bool

	// Scan copies the current row into dest.
	Scan(dest ...any) error

	// Values returns the current row as a slice of values.
	Values() ([]any, error)
}
