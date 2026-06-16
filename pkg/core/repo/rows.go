package repo

// Rows abstracts a forward-only query result iterator.
// Callers must always Close the iterator and then inspect Err to detect late
// driver failures.
type Rows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
	Values() ([]any, error)
}
