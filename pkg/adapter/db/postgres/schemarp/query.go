package schemarp

import (
	"context"
	"fmt"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

func CreateSchema[Q postgres.Queryer](
	ctx context.Context, q Q, schema string,
) error {
	// This DDL statement does not support to take schema name
	// as a parameterized query, it is a trusted string.
	_, err := q.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, schema))
	return err
}

func CreateRoleIfNotExists[Q postgres.Queryer](
	ctx context.Context, q Q, role repo.Role, password string,
) error {
	// Although CREATE ROLE DDL statement does not support to take
	// role name as a parameterized query, it is a trusted string.
	_, err := q.Exec(ctx, fmt.Sprintf(`DO
$body$
BEGIN
    IF NOT EXISTS (
            SELECT *
            FROM pg_catalog.pg_roles
            WHERE rolname = '%s'
    ) THEN
        BEGIN
            CREATE ROLE %[1]s WITH LOGIN PASSWORD '%s';
        EXCEPTION WHEN duplicate_object THEN
            RAISE NOTICE '%%, skipping', SQLERRM USING ERRCODE=SQLSTATE;
        END;
    END IF;
END
$body$`, role, password))
	return err
}

func GrantPrivileges[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	schema string,
	role repo.Role,
) error {
	// Although this DDL statement does not support to take schema and
	// role names as a parameterized query, they are trusted strings.
	_, err := q.Exec(ctx, fmt.Sprintf(
		`GRANT ALL PRIVILEGES ON SCHEMA %s TO %s`, schema, role,
	))
	return err
}

func SetSearchPath[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	schema string,
	role repo.Role,
) error {
	// Although this DDL statement does not support to take schema and
	// role names as a parameterized query, they are trusted strings.
	_, err := q.Exec(
		ctx,
		fmt.Sprintf(`ALTER ROLE %s SET search_path = %s`, role, schema),
	)
	return err
}

func DropIfExists[Q postgres.Queryer](
	ctx context.Context, q Q, schema string,
) error {
	// Although this DDL statement does not support to take schema name
	// as a parameterized query, it is a trusted string.
	_, err := q.Exec(
		ctx,
		fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, schema),
	)
	return err
}
