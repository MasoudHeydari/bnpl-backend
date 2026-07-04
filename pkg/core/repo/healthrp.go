package repo

import "context"

// HealthQueryer verifies database reachability for readiness probes.
type HealthQueryer interface {
	// Ping verifies that the database is reachable.
	Ping(ctx context.Context) error
}

// HealthConnQueryer exposes health checks on a plain connection.
type HealthConnQueryer interface {
	// HealthQueryer provides shared health probe query operations.
	HealthQueryer
}

// Health creates connection-bound query helpers for health probes.
type Health interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) HealthConnQueryer
}
