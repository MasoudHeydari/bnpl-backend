// Package repo defines repository and database-port abstractions used by the
// core and application layers. It belongs to the core domain boundary layer:
// interfaces point inward to business workflows while infrastructure adapters
// implement them outward. The package intentionally contains contracts only.
package repo

import "context"

// TxHandler receives a transaction scoped to the lifetime of a single Conn.Tx
// call. Returning an error aborts the transaction.
type TxHandler func(context.Context, Tx) error

// Conn abstracts a database connection that can execute queries directly or
// open transactions. Implementations are expected to be request-scoped rather
// than shared freely across concurrent callers unless documented otherwise.
type Conn interface {
	Queryer

	// Tx executes handler inside a database transaction.
	// The implementation is responsible for commit and rollback semantics.
	Tx(ctx context.Context, handler TxHandler) error

	// IsConn method prevents a non-Conn object to mistakenly implement
	// the Conn interface.
	IsConn()
}
