package repo

type Rows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
	Values() ([]any, error)
}
