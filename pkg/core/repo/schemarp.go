package repo

import "context"

type SchemaInitializer interface {
	InitDevSchema(ctx context.Context) error
	InitProdSchema(ctx context.Context) error
}

type Schema interface {
	Conn(Conn) SchemaConnQueryer
	Tx(Tx) SchemaTxQueryer
}

type SchemaConnQueryer interface {
	SchemaQueryer
}

type SchemaTxQueryer interface {
	SchemaQueryer
}

type SchemaQueryer interface {
	CreateSchema(ctx context.Context, schema string) error
	CreateRoleIfNotExists(ctx context.Context, role Role) error
	GrantPrivileges(ctx context.Context, schema string, role Role) error
	SetSearchPath(ctx context.Context, schema string, role Role) error
	DropIfExists(ctx context.Context, schema string) error
}
