package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type ConnHandler = repo.ConnHandler

type Pool struct {
	*pgxpool.Pool
}

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

func NoOpConnHandler(context.Context, repo.Conn) error {
	return nil
}

func (p *Pool) Conn(ctx context.Context, f repo.ConnHandler) error {
	return p.Pool.AcquireFunc(ctx, func(c *pgxpool.Conn) error {
		cc := &Conn{Conn: c}
		return f(ctx, cc)
	})
}

// Close closes all connections in the pool and rejects future Acquire calls. Blocks until all connections are returned
// to pool and closed.
func (p *Pool) Close() error {
	p.Pool.Close()
	return nil
}
