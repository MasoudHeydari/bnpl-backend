package schemarp

import (
	"context"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type Repo struct {
}

func New() *Repo {
	return &Repo{}
}

type connQueryer struct {
	*postgres.Conn
}

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
	ctx context.Context, role repo.Role,
) error {
	return CreateRoleIfNotExists(ctx, cq.Conn, role)
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
	ctx context.Context, role repo.Role,
) error {
	return CreateRoleIfNotExists(ctx, tq.Tx, role)
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
