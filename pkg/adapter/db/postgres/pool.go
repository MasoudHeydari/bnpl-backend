package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
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
	const op = "postgres.NewPool"

	pgPool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, fmt.Errorf("%s: pgxpool.NewWithConfig: %w", op, err)
	}

	pool := &Pool{Pool: pgPool}
	err = pool.Conn(ctx, NoOpConnHandler)
	if err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("%s: testing connection: %w", op, err)
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
//
// Acquire failures that wrap pgconn.ConnectError are masked so DSN identity
// fields (user/database) are never returned to callers or API clients.
func (p *Pool) Conn(ctx context.Context, f repo.ConnHandler) error {
	err := p.AcquireFunc(ctx, func(c *pgxpool.Conn) error {
		cc := &Conn{Conn: c}
		return f(ctx, cc)
	})

	return MapError(err)
}

// Close closes all connections in the pool and rejects future Acquire calls.
func (p *Pool) Close() error {
	p.Pool.Close()
	return nil
}
