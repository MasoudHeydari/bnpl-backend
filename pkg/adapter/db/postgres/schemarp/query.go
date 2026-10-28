package schemarp

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreateSchema creates schema using trusted identifier interpolation for PostgreSQL DDL.
func CreateSchema[Q postgres.Queryer](
	ctx context.Context, q Q, schema string,
) error {
	// PostgreSQL DDL does not accept schema names as bind parameters.
	_, err := q.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %s AUTHORIZATION %s`, pgx.Identifier{schema}.Sanitize(), pgx.Identifier{string(repo.OwnerRole)}.Sanitize()))
	return err
}

// CreateRoleIfNotExists provisions only typed managed roles. Existing passwords
// are retained: password rotation is an explicit separate operator action. Admin
// pools MUST disable query tracing; PostgreSQL statement logging must be redacted
// for provisioning sessions. Errors deliberately exclude credential-bearing SQL.
func CreateRoleIfNotExists[Q postgres.Queryer](ctx context.Context, q Q, role repo.Role, password string) error {
	if role != repo.OwnerRole && role != repo.MigratorRole && role != repo.NormalRole {
		return fmt.Errorf("unsupported provisioning role")
	}
	type result struct {
		Exists bool `db:"exists"`
	}
	present, e := postgres.QueryOne[result](ctx, q, "inspect managed role", `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=$1) AS exists`, string(role))
	if e != nil {
		return e
	}
	ident := pgx.Identifier{string(role)}.Sanitize()
	if !present.Exists {
		attributes := "NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"
		if role != repo.OwnerRole {
			if password == "" {
				return fmt.Errorf("managed login password is required")
			}
			attributes = "LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD " + passwordLiteral(password)
		}
		if _, e = q.Exec(ctx, "CREATE ROLE "+ident+" "+attributes); e != nil {
			return fmt.Errorf("managed role provisioning failed; inspect restricted database diagnostics")
		}
	}
	// Never silently downgrade an existing privileged/shared login.
	type safe struct {
		Safe bool `db:"safe"`
	}
	ok, e := postgres.QueryOne[safe](ctx, q, "validate managed role attributes", `SELECT NOT(rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls) AND rolcanlogin=$2 AND NOT EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles parent ON parent.oid=m.roleid WHERE m.member=r.oid AND ($1<>$3 OR parent.rolname<>$4 OR m.admin_option)) AS safe FROM pg_catalog.pg_roles r WHERE rolname=$1`, string(role), role != repo.OwnerRole, string(repo.MigratorRole), string(repo.OwnerRole))
	if e != nil {
		return e
	}
	if !ok.Safe {
		return fmt.Errorf("unsafe managed role attributes for %s; explicit operator remediation required", role)
	}
	if role != repo.OwnerRole {
		if _, e = q.Exec(ctx, "ALTER ROLE "+ident+" NOINHERIT"); e != nil {
			return e
		}
	}
	return nil
}

// GrantPrivileges establishes only explicit migration ownership access. Business
// DML is installed by versioned migration, never by blanket bootstrap grants.
func GrantPrivileges[Q postgres.Queryer](ctx context.Context, q Q, schema string, role repo.Role) error {
	if schema != "bnpl" || role != repo.MigratorRole {
		return fmt.Errorf("unsupported bootstrap grant")
	}
	_, e := q.Exec(ctx, "GRANT "+pgx.Identifier{string(repo.OwnerRole)}.Sanitize()+" TO "+pgx.Identifier{string(role)}.Sanitize())
	if e != nil {
		return e
	}
	type platform struct {
		Safe bool `db:"safe"`
	}
	safety, err := postgres.QueryOne[platform](ctx, q, "verify platform database DDL boundary", `SELECT NOT has_database_privilege($1,current_database(),'CREATE') AND NOT has_database_privilege($1,current_database(),'TEMP') AND NOT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='public' AND has_schema_privilege($1,oid,'CREATE')) AS safe`, string(repo.NormalRole))
	if err != nil {
		return err
	}
	if !safety.Safe {
		return fmt.Errorf("platform database/public-schema CREATE or TEMP reaches Runtime; DBA must explicitly tighten target database ACL before bootstrap")
	}
	// PostgreSQL 16+ stores inheritance/SET rights on each membership. NOINHERIT
	// alone does not repair an older membership's INHERIT option.
	type version struct {
		Number int `db:"number"`
	}
	v, err := postgres.QueryOne[version](ctx, q, "inspect role membership version", `SELECT current_setting('server_version_num')::int AS number`)
	if err != nil {
		return err
	}
	if v.Number >= 160000 {
		for _, option := range []string{"INHERIT FALSE", "SET TRUE"} {
			if _, err = q.Exec(ctx, "GRANT "+pgx.Identifier{string(repo.OwnerRole)}.Sanitize()+" TO "+pgx.Identifier{string(role)}.Sanitize()+" WITH "+option); err != nil {
				return err
			}
		}
	}
	// Creating a schema AUTHORIZATION owner and transferring ownership requires
	// the provisioning administrator to be able to SET ROLE owner as well.
	_, e = q.Exec(ctx, "GRANT "+pgx.Identifier{string(repo.OwnerRole)}.Sanitize()+" TO CURRENT_USER")
	if e != nil {
		return e
	}
	if v.Number >= 160000 {
		for _, option := range []string{"SET TRUE", "INHERIT TRUE"} {
			if _, e = q.Exec(ctx, "GRANT "+pgx.Identifier{string(repo.OwnerRole)}.Sanitize()+" TO CURRENT_USER WITH "+option); e != nil {
				return e
			}
		}
	}
	return nil
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
		fmt.Sprintf(`ALTER ROLE %s SET search_path = %s, pg_catalog`, pgx.Identifier{string(role)}.Sanitize(), pgx.Identifier{schema}.Sanitize()),
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

// passwordLiteral escapes DDL input independently of server string settings.
func passwordLiteral(password string) string {
	return "E'" + strings.ReplaceAll(strings.ReplaceAll(password, "\\", "\\\\"), "'", "''") + "'"
}
