package schemarp

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreateSchema creates schema using trusted identifier interpolation for PostgreSQL DDL.
func CreateSchema[Q postgres.Queryer](
	ctx context.Context, q Q, schema string,
) error {
	// PostgreSQL DDL does not accept schema names as bind parameters.
	_, err := q.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, schema))
	return err
}

// CreateRoleIfNotExists creates role with password when the role is not already present.
// Role and password are interpolated into trusted SQL because this DDL form cannot bind identifiers.
// Password may appear in PostgreSQL logs when statement logging is enabled.
func CreateRoleIfNotExists[Q postgres.Queryer](
	ctx context.Context, q Q, role repo.Role, password string,
) error {
	// Role name is a trusted application constant; password is from configuration.
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

// GrantPrivileges grants all privileges on schema to role using trusted identifier interpolation.
func GrantPrivileges[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	schema string,
	role repo.Role,
) error {
	// Schema and role are trusted application-controlled identifiers.
	_, err := q.Exec(ctx, fmt.Sprintf(
		`GRANT ALL PRIVILEGES ON SCHEMA %s TO %s`, schema, role,
	))
	return err
}

// SetSearchPath sets role's default search_path to schema.
func SetSearchPath[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	schema string,
	role repo.Role,
) error {
	// Schema and role are trusted application-controlled identifiers.
	_, err := q.Exec(
		ctx,
		fmt.Sprintf(`ALTER ROLE %s SET search_path = %s`, role, schema),
	)
	return err
}

// Exists reports whether schema is present in pg_namespace.
func Exists[Q postgres.Queryer](
	ctx context.Context, q Q, schema string,
) (bool, error) {
	const query = `
SELECT EXISTS (
	SELECT 1
	FROM pg_catalog.pg_namespace
	WHERE nspname = $1
) AS exists
`

	type result struct {
		Exists bool `db:"exists"`
	}

	value, err := postgres.QueryOne[result](
		ctx,
		q,
		"check schema exists",
		query,
		schema,
	)
	if err != nil {
		return false, err
	}

	return value.Exists, nil
}
