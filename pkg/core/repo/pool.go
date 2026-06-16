package repo

import "context"

// ConnHandler receives a leased connection from Pool.Conn.
// The connection is valid only for the duration of the callback.
type ConnHandler func(context.Context, Conn) error

// Pool abstracts a concurrency-safe connection pool.
// Implementations may maintain network resources and must be closed when the
// application no longer needs them.
type Pool interface {
	// Conn leases a connection for the duration of handler.
	// Implementations must release the connection after handler returns.
	Conn(ctx context.Context, handler ConnHandler) error

	// Close releases pool-owned resources and rejects future leases.
	Close() error
}
