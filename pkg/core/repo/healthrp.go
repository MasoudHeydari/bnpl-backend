package repo

import "context"

// HealthQueryer verifies database reachability for readiness probes.
type HealthQueryer interface {
	Ping(ctx context.Context) error
}

// HealthConnQueryer exposes health checks on a plain connection.
type HealthConnQueryer interface {
	HealthQueryer
}

// Health creates connection-bound query helpers for health probes.
type Health interface {
	Conn(Conn) HealthConnQueryer
}
