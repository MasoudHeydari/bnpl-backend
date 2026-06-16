// Package postgres provides PostgreSQL implementations of the repository port
// abstractions. It belongs to the infrastructure layer and adapts pgx/pgxpool
// primitives to the repository contracts defined in the core layer.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

// ConnHandler aliases the core pooled-connection callback type for local
// adapter use.
type ConnHandler = repo.ConnHandler

// Pool adapts pgxpool.Pool to the core repository Pool interface.
// Values are safe for concurrent use after construction.
type Pool struct {
	*pgxpool.Pool
}

// NewPool constructs and validates a PostgreSQL connection pool from pgConfig.
// The function performs an initial connection lease to fail fast on invalid
// configuration or unreachable databases.
func NewPool(ctx context.Context, pgConfig *pgxpool.Config) (*Pool, error) {
	pgPool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}
	pool := &Pool{Pool: pgPool}
	err = pool.Conn(ctx, NoOpConnHandler)
	if err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("testing connection: %w", err)
	}
	return pool, nil
}

// NoOpConnHandler is a connection handler that performs no work.
// It is used to validate pool connectivity during startup.
func NoOpConnHandler(context.Context, repo.Conn) error {
	return nil
}

// Conn acquires a pooled connection, wraps it in the repository adapter, and
// releases it after f returns.
func (p *Pool) Conn(ctx context.Context, f repo.ConnHandler) error {
	return p.AcquireFunc(ctx, func(c *pgxpool.Conn) error {
		cc := &Conn{Conn: c}
		return f(ctx, cc)
	})
}

// Close closes all connections in the pool and rejects future Acquire calls.
func (p *Pool) Close() error {
	p.Pool.Close()
	return nil
}
