package schemarp

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// OwnershipChange is a non-secret, reviewable operation scoped to schema bnpl.
// There is deliberately no REASSIGN OWNED or cross-schema operation.
type OwnershipChange struct {
	Object string `db:"object"`
	Owner  string `db:"owner"`
	SQL    string `db:"sql"`
}

// OwnershipPlan inventories managed owners before any role or object change.
// Extension-owned or unsupported relations are blockers, never silently moved.
func OwnershipPlan[Q postgres.Queryer](ctx context.Context, q Q) ([]OwnershipChange, error) {
	type blocker struct {
		Name string `db:"name"`
	}
	blockers, e := postgres.QueryAll[blocker](ctx, q, "preflight ownership blockers", `
 SELECT c.relname AS name FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='bnpl' AND (c.relkind NOT IN ('r','p','i','I','S','v','m','c') OR EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_class'::regclass AND d.objid=c.oid AND d.deptype='e'))
 UNION ALL SELECT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='bnpl' AND EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_proc'::regclass AND d.objid=p.oid AND d.deptype='e')
 UNION ALL SELECT 'insufficient ownership authority for '||n.nspname||'.'||c.relname||'; operator must authorize scoped transfer from '||r.rolname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_roles r ON r.oid=c.relowner WHERE n.nspname='bnpl' AND NOT pg_has_role(current_user,c.relowner,'USAGE')
 UNION ALL SELECT 'insufficient schema ownership authority; operator must authorize transfer from '||r.rolname FROM pg_namespace n JOIN pg_roles r ON r.oid=n.nspowner WHERE n.nspname='bnpl' AND NOT pg_has_role(current_user,n.nspowner,'USAGE')
 UNION ALL SELECT 'Runtime can CREATE schemas through database/PUBLIC ACL; platform DBA must explicitly tighten the target database ACL' WHERE EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1) AND has_database_privilege($1,current_database(),'CREATE')
 UNION ALL SELECT 'database owner is runtime: transfer separately under platform administration' FROM pg_database d JOIN pg_roles r ON r.oid=d.datdba WHERE d.datname=current_database() AND r.rolname=$1`, string(repo.NormalRole))
	if e != nil {
		return nil, e
	}
	if len(blockers) > 0 {
		return nil, fmt.Errorf("ownership preflight blocker: %s", blockers[0].Name)
	}
	return postgres.QueryAll[OwnershipChange](ctx, q, "inventory managed ownership", `
 SELECT 'schema bnpl' AS object,r.rolname AS owner,format('ALTER SCHEMA bnpl OWNER TO %I',$1::text) AS sql FROM pg_namespace n JOIN pg_roles r ON r.oid=n.nspowner WHERE n.nspname='bnpl'
 UNION ALL SELECT n.nspname||'.'||c.relname,r.rolname,format('ALTER %s %I.%I OWNER TO %I',CASE c.relkind WHEN 'S' THEN 'SEQUENCE' WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW' ELSE 'TABLE' END,n.nspname,c.relname,$1::text) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_roles r ON r.oid=c.relowner WHERE n.nspname='bnpl' AND c.relkind IN ('r','p','S','v','m') AND NOT(c.relkind='S' AND EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_class'::regclass AND d.objid=c.oid AND d.deptype IN ('a','i')))
 UNION ALL SELECT n.nspname||'.'||p.proname||'('||pg_get_function_identity_arguments(p.oid)||')',r.rolname,format('ALTER %s %I.%I(%s) OWNER TO %I',CASE p.prokind WHEN 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END,n.nspname,p.proname,pg_get_function_identity_arguments(p.oid),$1::text) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace JOIN pg_roles r ON r.oid=p.proowner WHERE n.nspname='bnpl'
 UNION ALL SELECT n.nspname||'.'||t.typname,r.rolname,format('ALTER %s %I.%I OWNER TO %I',CASE t.typtype WHEN 'd' THEN 'DOMAIN' ELSE 'TYPE' END,n.nspname,t.typname,$1::text) FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace JOIN pg_roles r ON r.oid=t.typowner WHERE n.nspname='bnpl' AND t.typelem=0 AND (t.typrelid=0 OR EXISTS(SELECT 1 FROM pg_class c WHERE c.oid=t.typrelid AND c.relkind='c'))
 `, string(repo.OwnerRole))
}

// ApplyOwnership executes only the previously inventoried managed statements
// inside an operator-owned transaction while old writers are drained.
func ApplyOwnership(ctx context.Context, q *postgres.Tx, changes []OwnershipChange) error {
	for _, change := range changes {
		if _, e := q.Exec(ctx, change.SQL); e != nil {
			return fmt.Errorf("transfer managed owner of %s: %w", change.Object, postgres.MapError(e))
		}
	}
	// Legacy global/default grants are revoked only for the former managed creator
	// and the new owner, only in bnpl. No unrelated schemas are reassigned.
	for _, role := range []repo.Role{repo.NormalRole, repo.OwnerRole} {
		ident := pgx.Identifier{string(role)}.Sanitize()
		for _, kind := range []string{"TABLES", "SEQUENCES", "FUNCTIONS"} {
			if _, e := q.Exec(ctx, "ALTER DEFAULT PRIVILEGES FOR ROLE "+ident+" IN SCHEMA bnpl REVOKE ALL ON "+kind+" FROM PUBLIC, "+pgx.Identifier{string(repo.NormalRole)}.Sanitize()); e != nil {
				return e
			}
		}
	}
	return nil
}

// SecurityInventory excludes all password/role configuration fields. It is safe
// operator metadata for an upgrade plan, not a source of installed grants.
func SecurityInventory[Q postgres.Queryer](ctx context.Context, q Q) ([]string, error) {
	type row struct {
		Description string `db:"description"`
	}
	values, e := postgres.QueryAll[row](ctx, q, "inventory managed security", `
 SELECT format('role %s login=%s superuser=%s createrole=%s createdb=%s bypassrls=%s inherit=%s',rolname,rolcanlogin,rolsuper,rolcreaterole,rolcreatedb,rolbypassrls,rolinherit) AS description FROM pg_roles WHERE rolname IN ($1,$2,$3,$4)
 UNION ALL SELECT format('membership %s -> %s admin_option=%s',member.rolname,parent.rolname,m.admin_option) FROM pg_auth_members m JOIN pg_roles member ON member.oid=m.member JOIN pg_roles parent ON parent.oid=m.roleid WHERE member.rolname IN ($1,$2,$3,$4)
 UNION ALL SELECT format('ACL bnpl.%s %s',c.relname,coalesce(c.relacl::text,'<owner/default>')) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='bnpl' AND c.relkind IN ('r','p','S','v','m')
 UNION ALL SELECT format('schema ACL bnpl %s',coalesce(nspacl::text,'<owner/default>')) FROM pg_namespace WHERE nspname='bnpl'
 UNION ALL SELECT format('function ACL bnpl.%s %s',p.proname,coalesce(p.proacl::text,'<owner/default>')) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='bnpl'
 UNION ALL SELECT format('default ACL role=%s schema=%s kind=%s %s',r.rolname,coalesce(n.nspname,'<global>'),d.defaclobjtype,d.defaclacl::text) FROM pg_default_acl d JOIN pg_roles r ON r.oid=d.defaclrole LEFT JOIN pg_namespace n ON n.oid=d.defaclnamespace WHERE r.rolname IN ($2,$4) AND (n.nspname='bnpl' OR d.defaclnamespace=0)
 UNION ALL SELECT format('column ACL bnpl.%s.%s %s',c.relname,a.attname,a.attacl::text) FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='bnpl' AND a.attacl IS NOT NULL
 `, string(repo.AdminRole), string(repo.OwnerRole), string(repo.MigratorRole), string(repo.NormalRole))
	if e != nil {
		return nil, e
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = v.Description
	}
	return out, nil
}
