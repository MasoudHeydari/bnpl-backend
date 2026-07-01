package healthrp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core health repository contract.
var _ repo.Health = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the core health repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL health repository.
func New() *Repo {
	return &Repo{}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core health connection queryer contract.
var _ repo.HealthConnQueryer = (*connQueryer)(nil)

// Conn binds the health repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.HealthConnQueryer {
	return connQueryer{Conn: c.(*postgres.Conn)}
}

func (cq connQueryer) Ping(ctx context.Context) error {
	return Ping(ctx, cq.Conn)
}
