// Package schemarp implements PostgreSQL schema administration queries.
// It belongs to the infrastructure layer and backs the initialization use case
// with concrete SQL for schema, role, and privilege management.
package schemarp

import (
	"context"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

// Repo is the PostgreSQL implementation of the core schema repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL schema repository.
func New() *Repo {
	return &Repo{}
}

type connQueryer struct {
	*postgres.Conn
}

// Conn binds the schema repository to a non-transactional connection scope.
func (schema *Repo) Conn(c repo.Conn) repo.SchemaConnQueryer {
	cc := c.(*postgres.Conn)
	return connQueryer{Conn: cc}
}

func (cq connQueryer) CreateSchema(
	ctx context.Context, schema string,
) error {
	return CreateSchema(ctx, cq.Conn, schema)
}

func (cq connQueryer) CreateRoleIfNotExists(
	ctx context.Context, role repo.Role, password string,
) error {
	return CreateRoleIfNotExists(ctx, cq.Conn, role, password)
}

func (cq connQueryer) GrantPrivileges(
	ctx context.Context, schema string, role repo.Role,
) error {
	return GrantPrivileges(ctx, cq.Conn, schema, role)
}

func (cq connQueryer) SetSearchPath(
	ctx context.Context, schema string, role repo.Role,
) error {
	return SetSearchPath(ctx, cq.Conn, schema, role)
}

func (cq connQueryer) DropIfExists(
	ctx context.Context, schema string,
) error {
	return DropIfExists(ctx, cq.Conn, schema)
}

type txQueryer struct {
	*postgres.Tx
}

// Tx binds the schema repository to a transaction scope.
func (schema *Repo) Tx(tx repo.Tx) repo.SchemaTxQueryer {
	tt := tx.(*postgres.Tx)
	return txQueryer{Tx: tt}
}

func (tq txQueryer) CreateSchema(
	ctx context.Context, schema string,
) error {
	return CreateSchema(ctx, tq.Tx, schema)
}

func (tq txQueryer) CreateRoleIfNotExists(
	ctx context.Context, role repo.Role, password string,
) error {
	return CreateRoleIfNotExists(ctx, tq.Tx, role, password)
}

func (tq txQueryer) GrantPrivileges(
	ctx context.Context, schema string, role repo.Role,
) error {
	return GrantPrivileges(ctx, tq.Tx, schema, role)
}

func (tq txQueryer) SetSearchPath(
	ctx context.Context, schema string, role repo.Role,
) error {
	return SetSearchPath(ctx, tq.Tx, schema, role)
}

func (tq txQueryer) DropIfExists(
	ctx context.Context, schema string,
) error {
	return DropIfExists(ctx, tq.Tx, schema)
}
