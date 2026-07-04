package schemarp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core schema repository contract.
var _ repo.Schema = (*Repo)(nil)

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

// Verify connQueryer implements the core schema connection queryer contract.
var _ repo.SchemaConnQueryer = (*connQueryer)(nil)

// Conn binds the schema repository to a non-transactional connection scope.
func (schema *Repo) Conn(c repo.Conn) repo.SchemaConnQueryer {
	cc := postgres.MustConn(c)
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

func (cq connQueryer) Exists(
	ctx context.Context, schema string,
) (bool, error) {
	return Exists(ctx, cq.Conn, schema)
}

func (cq connQueryer) DropIfExists(
	ctx context.Context, schema string,
) error {
	return DropIfExists(ctx, cq.Conn, schema)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core schema transaction queryer contract.
var _ repo.SchemaTxQueryer = (*txQueryer)(nil)

// Tx binds the schema repository to a transaction scope.
func (schema *Repo) Tx(tx repo.Tx) repo.SchemaTxQueryer {
	tt := postgres.MustTx(tx)
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

func (tq txQueryer) Exists(
	ctx context.Context, schema string,
) (bool, error) {
	return Exists(ctx, tq.Tx, schema)
}

func (tq txQueryer) DropIfExists(
	ctx context.Context, schema string,
) error {
	return DropIfExists(ctx, tq.Tx, schema)
}
