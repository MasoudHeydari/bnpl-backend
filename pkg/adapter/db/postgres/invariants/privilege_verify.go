package invariants

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// VerifyRuntime checks identity, managed trigger integrity and the complete
// effective privilege contract before any listener or worker is started.
func VerifyRuntime(ctx context.Context, p repo.Pool) error {
	e := p.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		type identity struct{ Current, Session string }
		v, e := postgres.QueryOne[identity](ctx, postgres.MustConn(c), "verify runtime identity", `SELECT current_user AS current, session_user AS session`)
		if e != nil {
			return e
		}
		if v.Current != string(repo.NormalRole) || v.Session != string(repo.NormalRole) {
			return fmt.Errorf("runtime database connection must authenticate and execute as %s", repo.NormalRole)
		}
		return nil
	})
	if e != nil {
		return e
	}
	if e = Verify(ctx, p); e != nil {
		return e
	}
	return VerifyPrivileges(ctx, p)
}

// VerifyPrivileges is read-only and can run as bnpl or the administrative owner.
// Effective checks include PUBLIC and inherited rights, not just direct ACLs.
func VerifyPrivileges(ctx context.Context, p repo.Pool) error {
	return p.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.ReadSnapshot(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := postgres.MustTx(tx)
			type fact struct {
				Name string `db:"name"`
				Safe bool   `db:"safe"`
			}
			facts, e := postgres.QueryAll[fact](ctx, q, "verify database role and ownership safety", `
 SELECT 'runtime attributes/membership' AS name, EXISTS(SELECT 1 FROM pg_roles r WHERE r.rolname=$1 AND r.rolcanlogin AND NOT(r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls OR r.rolreplication) AND NOT EXISTS(SELECT 1 FROM pg_auth_members m WHERE m.member=r.oid)) AS safe
 UNION ALL SELECT 'owner attributes', EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$2 AND NOT(rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication))
 UNION ALL SELECT 'migrator attributes/membership', EXISTS(SELECT 1 FROM pg_roles r WHERE r.rolname=$3 AND r.rolcanlogin AND NOT(r.rolinherit OR r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls OR r.rolreplication) AND EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles o ON o.oid=m.roleid WHERE m.member=r.oid AND o.rolname=$2) AND NOT EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles o ON o.oid=m.roleid WHERE m.member=r.oid AND (o.rolname<>$2 OR m.admin_option OR coalesce((to_jsonb(m)->>'inherit_option')::boolean,false) OR NOT coalesce((to_jsonb(m)->>'set_option')::boolean,true))))
 UNION ALL SELECT 'admin role', EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$4 AND rolcanlogin)
 UNION ALL SELECT 'schema ownership', EXISTS(SELECT 1 FROM pg_namespace n JOIN pg_roles r ON r.oid=n.nspowner WHERE n.nspname='bnpl' AND r.rolname=$2)
 UNION ALL SELECT 'schema usage/create', has_schema_privilege($1,'bnpl','USAGE') AND NOT has_schema_privilege($1,'bnpl','CREATE')
 UNION ALL SELECT 'database create', NOT has_database_privilege($1,current_database(),'CREATE')
 UNION ALL SELECT 'public schema create', NOT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='public' AND has_schema_privilege($1,oid,'CREATE'))
 UNION ALL SELECT 'relation ownership', NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_roles r ON r.oid=c.relowner WHERE n.nspname='bnpl' AND r.rolname<>$2)
 UNION ALL SELECT 'function ownership/security', NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace JOIN pg_roles r ON r.oid=p.proowner WHERE n.nspname='bnpl' AND (r.rolname<>$2 OR p.prosecdef))
 UNION ALL SELECT 'type ownership', NOT EXISTS(SELECT 1 FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace JOIN pg_roles r ON r.oid=t.typowner WHERE n.nspname='bnpl' AND r.rolname<>$2)
 UNION ALL SELECT 'owner membership', NOT EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname=$2)
 UNION ALL SELECT 'runtime delegation privileges', NOT EXISTS(
 SELECT 1 FROM (
 SELECT x.grantee,x.is_grantable FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace CROSS JOIN LATERAL aclexplode(c.relacl) x WHERE n.nspname='bnpl'
 UNION ALL SELECT x.grantee,x.is_grantable FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace CROSS JOIN LATERAL aclexplode(a.attacl) x WHERE n.nspname='bnpl'
 UNION ALL SELECT x.grantee,x.is_grantable FROM pg_namespace n CROSS JOIN LATERAL aclexplode(n.nspacl) x WHERE n.nspname='bnpl'
 UNION ALL SELECT x.grantee,x.is_grantable FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace CROSS JOIN LATERAL aclexplode(p.proacl) x WHERE n.nspname='bnpl') rights
 WHERE rights.is_grantable AND (rights.grantee=0 OR rights.grantee=(SELECT oid FROM pg_roles WHERE rolname=$1)))
 UNION ALL SELECT 'future object default deny', NOT EXISTS(SELECT 1 FROM pg_default_acl d JOIN pg_roles r ON r.oid=d.defaclrole LEFT JOIN pg_namespace n ON n.oid=d.defaclnamespace CROSS JOIN LATERAL aclexplode(d.defaclacl) a WHERE r.rolname=$2 AND (n.nspname='bnpl' OR d.defaclnamespace=0) AND a.grantee<>r.oid AND a.privilege_type IN ('SELECT','INSERT','UPDATE','DELETE','TRUNCATE','REFERENCES','TRIGGER','USAGE','EXECUTE'))
 UNION ALL SELECT 'future function PUBLIC execute revoked', EXISTS(SELECT 1 FROM pg_default_acl d JOIN pg_roles r ON r.oid=d.defaclrole WHERE r.rolname=$2 AND d.defaclnamespace=0 AND d.defaclobjtype='f' AND NOT EXISTS(SELECT 1 FROM aclexplode(d.defaclacl) a WHERE a.grantee=0 AND a.privilege_type='EXECUTE'))
 `, string(repo.NormalRole), string(repo.OwnerRole), string(repo.MigratorRole), string(repo.AdminRole))
			if e != nil {
				return fmt.Errorf("privilege verification cannot inspect role/ownership catalogs: %w", e)
			}
			var failures []error
			for _, f := range facts {
				if !f.Safe {
					failures = append(failures, fmt.Errorf("database safety drift: %s; run DB Admin preflight and reviewed reconciliation", f.Name))
				}
			}
			type relation struct {
				Name string `db:"name"`
				Kind string `db:"kind"`
			}
			relations, e := postgres.QueryAll[relation](ctx, q, "inventory managed relations", `SELECT c.relname AS name,c.relkind::text AS kind FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='bnpl' AND c.relkind IN ('r','p','v','m','f','S') ORDER BY c.relname`)
			if e != nil {
				return e
			}
			policies := map[string]TablePrivilegePolicy{}
			for _, v := range PrivilegeRegistry() {
				policies[v.Table] = v
			}
			type allowed struct {
				Allowed bool `db:"allowed"`
			}
			for _, r := range relations {
				policy, known := policies[r.Name]
				if r.Kind == "S" {
					for _, op := range []string{"USAGE", "SELECT", "UPDATE"} {
						v, e := postgres.QueryOne[allowed](ctx, q, "verify sequence privilege", `SELECT has_sequence_privilege($1,$2,$3) AS allowed`, string(repo.NormalRole), pgx.Identifier{"bnpl", r.Name}.Sanitize(), op)
						if e != nil {
							return e
						}
						if v.Allowed {
							failures = append(failures, fmt.Errorf("unexpected sequence privilege %s %s", r.Name, op))
						}
					}
					continue
				}
				if !known {
					failures = append(failures, fmt.Errorf("unregistered managed relation %s; add reviewed policy and migration", r.Name))
				}
				object := pgx.Identifier{"bnpl", r.Name}.Sanitize()
				for _, op := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
					v, e := postgres.QueryOne[allowed](ctx, q, "verify effective table privilege", `SELECT has_table_privilege($1,$2,$3) AS allowed`, string(repo.NormalRole), object, op)
					if e != nil {
						return e
					}
					want := known && slices.Contains(policy.Operations, Operation(op))
					if v.Allowed != want {
						failures = append(failures, fmt.Errorf("table privilege drift bnpl.%s %s: allowed=%t expected=%t", r.Name, op, v.Allowed, want))
					}
				}
				type column struct {
					Name string `db:"name"`
				}
				columns, e := postgres.QueryAll[column](ctx, q, "inventory managed columns", `SELECT a.attname AS name FROM pg_attribute a WHERE a.attrelid=$1::regclass AND a.attnum>0 AND NOT a.attisdropped`, object)
				if e != nil {
					return e
				}
				for _, required := range policy.UpdateColumns {
					found := false
					for _, actual := range columns {
						if actual.Name == required {
							found = true
						}
					}
					if !found {
						failures = append(failures, fmt.Errorf("required writable column missing bnpl.%s.%s", r.Name, required))
					}
				}
				for _, col := range columns {
					for _, op := range []string{"SELECT", "INSERT", "UPDATE", "REFERENCES"} {
						v, e := postgres.QueryOne[allowed](ctx, q, "verify effective column privilege", `SELECT has_column_privilege($1,$2,$3,$4) AS allowed`, string(repo.NormalRole), object, col.Name, op)
						if e != nil {
							return e
						}
						want := known && (slices.Contains(policy.Operations, Operation(op)) || (op == "UPDATE" && slices.Contains(policy.UpdateColumns, col.Name)))
						if v.Allowed != want {
							failures = append(failures, fmt.Errorf("column privilege drift bnpl.%s.%s %s: allowed=%t expected=%t", r.Name, col.Name, op, v.Allowed, want))
						}
					}
				}
				delete(policies, r.Name)
			}
			for name := range policies {
				failures = append(failures, fmt.Errorf("registered table bnpl.%s missing", name))
			}
			// Managed trigger functions are the only callable functions in the schema.
			type function struct {
				Name       string `db:"name"`
				Allowed    bool   `db:"allowed"`
				NArgs      int    `db:"nargs"`
				ReturnType string `db:"return_type"`
			}
			functions, e := postgres.QueryAll[function](ctx, q, "verify function execute privileges", `SELECT p.proname AS name,p.pronargs AS nargs,p.prorettype::regtype::text AS return_type,has_function_privilege($1,p.oid,'EXECUTE') AS allowed FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='bnpl'`, string(repo.NormalRole))
			if e != nil {
				return e
			}
			for _, f := range functions {
				want := false
				for _, v := range Registry() {
					if v.Function == f.Name && f.NArgs == 0 && f.ReturnType == "trigger" {
						want = true
					}
				}
				if f.Allowed != want {
					failures = append(failures, fmt.Errorf("function execute drift bnpl.%s", f.Name))
				}
			}
			// Initial-schema privileges are checked against actual PostgreSQL
			// effective rights above, not obsolete pre-release migration records.
			type compatible struct {
				Safe bool `db:"safe"`
			}
			h, e := postgres.QueryOne[compatible](ctx, q, "verify active permission history schema", `SELECT EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid='bnpl.merchant_terminal_permissions'::regclass AND a.attname='revoked_at' AND a.atttypid='timestamptz'::regtype AND NOT a.attnotnull AND NOT a.attisdropped) AND EXISTS(SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='bnpl' AND c.relname='uq_merchant_terminal_permissions_active' AND i.indisunique AND i.indisvalid AND i.indisready AND replace(replace(pg_get_expr(i.indpred,i.indrelid),'(',''),')','')='revoked_at IS NULL' AND i.indkey::text=(SELECT a.attnum::text||' '||b.attnum::text FROM pg_attribute a JOIN pg_attribute b ON b.attrelid=a.attrelid WHERE a.attrelid='bnpl.merchant_terminal_permissions'::regclass AND a.attname='merchant_terminal_id' AND b.attname='permission')) AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='bnpl.merchant_terminal_permissions'::regclass AND conname='uq_merchant_terminal_permissions_terminal_permission') AS safe`)
			if e != nil || !h.Safe {
				failures = append(failures, fmt.Errorf("incompatible terminal permission history schema; inspect the initial baseline"))
			}
			return errors.Join(failures...)
		})
	})
}

// MigrationFingerprint is shared by tests and administrative diagnostics.
func MigrationFingerprint(data []byte) string {
	s := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(s[:])
}
