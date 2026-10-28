// Package invariants describes and verifies migration-owned PostgreSQL integrity
// rules. Verification is read-only; it never installs or repairs database objects.
package invariants

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type Invariant struct {
	Name, Description, Owner, Schema, Table, Trigger, Function, Migration, Guidance, Code string
	EnabledState                                                                          string
	TriggerType                                                                           int16
	Deferred                                                                              bool
	FunctionSHA256                                                                        string
}

func Registry() []Invariant { return append([]Invariant(nil), managed...) }

// Verify runs before listeners/consumers using the runtime role, with no DDL.
func Verify(ctx context.Context, pool repo.Pool) error { return VerifySchema(ctx, pool, "bnpl") }

// VerifySchema also supports isolated integration-test schemas. Definitions are
// normalized only for their known schema prefix; PostgreSQL rendering is not hashed.
func VerifySchema(ctx context.Context, pool repo.Pool, schema string) error {
	return pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.ReadSnapshot(ctx, func(ctx context.Context, tx repo.Tx) error {
			type migration struct {
				Checksum string `db:"checksum"`
				Dirty    bool   `db:"dirty"`
			}
			m, err := postgres.QueryOne[migration](ctx, postgres.MustTx(tx), "read managed invariant migration", `SELECT checksum,dirty FROM `+pgx.Identifier{schema}.Sanitize()+`.migrations WHERE name=$1`, Migration)
			if err != nil {
				return fmt.Errorf("managed invariants: migration %s absent/unreadable; run db migrate after integrity preflight: %w", Migration, err)
			}
			if m.Dirty || m.Checksum != MigrationSHA256 {
				return fmt.Errorf("managed invariants: migration %s dirty or incompatible checksum; verify deployment history", Migration)
			}
			type compatibility struct {
				Compatible bool `db:"compatible"`
			}
			column, e := postgres.QueryOne[compatibility](ctx, postgres.MustTx(tx), "verify installment parent revision", `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace JOIN pg_catalog.pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum WHERE n.nspname=$1 AND c.relname='installments' AND a.attname='financial_revision' AND NOT a.attisdropped AND a.atttypid='bigint'::regtype AND a.attnotnull AND pg_get_expr(d.adbin,d.adrelid) IN ('0','0::bigint')) AS compatible`, schema)
			if e != nil || !column.Compatible {
				return fmt.Errorf("managed invariants: incompatible installment financial revision schema: %v", e)
			}
			marker, e := postgres.QueryOne[compatibility](ctx, postgres.MustTx(tx), "verify payment snapshot ownership", `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace JOIN pg_catalog.pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum WHERE n.nspname=$1 AND c.relname='payment_attempts' AND a.attname='snapshot_xid' AND NOT a.attisdropped AND a.atttypid='xid8'::regtype AND a.attnotnull AND pg_get_expr(d.adbin,d.adrelid)='pg_current_xact_id()') AS compatible`, schema)
			if e != nil || !marker.Compatible {
				return fmt.Errorf("managed invariants: incompatible payment snapshot ownership schema: %v", e)
			}
			authority, e := postgres.QueryOne[compatibility](ctx, postgres.MustTx(tx), "verify financial outcome metadata", `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace JOIN pg_catalog.pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum WHERE n.nspname=$1 AND c.relname='payment_attempts' AND a.attname='financial_outcome_resolved' AND NOT a.attisdropped AND a.atttypid='boolean'::regtype AND a.attnotnull AND pg_get_expr(d.adbin,d.adrelid) IN ('false','false::boolean')) AS compatible`, schema)
			if e != nil || !authority.Compatible {
				return fmt.Errorf("managed invariants: incompatible financial outcome metadata: %v", e)
			}
			clean, e := postgres.QueryOne[compatibility](ctx, postgres.MustTx(tx), "verify clean migration chain", `SELECT NOT EXISTS(SELECT 1 FROM `+pgx.Identifier{schema}.Sanitize()+`.migrations WHERE dirty) AS compatible`)
			if e != nil || !clean.Compatible {
				return fmt.Errorf("managed invariants: migration chain is dirty/unreadable: %v", e)
			}
			var failures []error
			for _, in := range managed {
				row, e := postgres.QueryOne[catalog](ctx, postgres.MustTx(tx), "verify managed database invariant", catalogSQL, schema, in.Table, in.Trigger)
				if e != nil {
					failures = append(failures, fmt.Errorf("%s: required trigger %s.%s missing/unreadable: %w", in.Name, schema, in.Trigger, e))
					continue
				}
				sum := sha256.Sum256([]byte(strings.ReplaceAll(row.Body, schema+".", "bnpl.")))
				if row.Enabled != in.EnabledState || row.TriggerType != in.TriggerType || row.Function != in.Function || row.FunctionSchema != schema || row.Deferred != in.Deferred || row.Deferrable != in.Deferred || row.Arguments != "" || row.Columns != "" || row.Qualified || row.SecurityDefiner || row.Language != "plpgsql" || row.NArgs != 0 || row.ReturnType != "trigger" || row.Volatility != "v" || row.Config != "search_path=pg_catalog, "+schema || hex.EncodeToString(sum[:]) != in.FunctionSHA256 {
					failures = append(failures, fmt.Errorf("%s: trigger/function drift or disabled enforcement (%s.%s); inspect catalogs and restore through a reviewed migration", in.Name, schema, in.Trigger))
				}
			}
			return errors.Join(failures...)
		})
	})
}

type catalog struct {
	Enabled         string `db:"enabled"`
	TriggerType     int16  `db:"trigger_type"`
	Function        string `db:"function"`
	FunctionSchema  string `db:"function_schema"`
	Deferrable      bool   `db:"deferrable"`
	Deferred        bool   `db:"deferred"`
	Arguments       string `db:"arguments"`
	Columns         string `db:"columns"`
	Qualified       bool   `db:"qualified"`
	Body            string `db:"body"`
	SecurityDefiner bool   `db:"security_definer"`
	Language        string `db:"language"`
	NArgs           int16  `db:"nargs"`
	ReturnType      string `db:"return_type"`
	Volatility      string `db:"volatility"`
	Config          string `db:"config"`
}

const catalogSQL = `SELECT t.tgenabled::text AS enabled,t.tgtype AS trigger_type,p.proname AS function,pn.nspname AS function_schema,t.tgdeferrable AS deferrable,t.tginitdeferred AS deferred,encode(t.tgargs,'hex') AS arguments,t.tgattr::text AS columns,(t.tgqual IS NOT NULL) AS qualified,p.prosrc AS body,p.prosecdef AS security_definer,l.lanname AS language,p.pronargs AS nargs,p.prorettype::regtype::text AS return_type,p.provolatile::text AS volatility,array_to_string(p.proconfig,',') AS config
FROM pg_catalog.pg_trigger t JOIN pg_catalog.pg_class c ON c.oid=t.tgrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace JOIN pg_catalog.pg_proc p ON p.oid=t.tgfoid JOIN pg_catalog.pg_namespace pn ON pn.oid=p.pronamespace JOIN pg_catalog.pg_language l ON l.oid=p.prolang WHERE n.nspname=$1 AND c.relname=$2 AND t.tgname=$3 AND NOT t.tgisinternal`
