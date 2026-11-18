package invariants

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrivilegeRegistryMatchesInitialSchema(t *testing.T) {
	baseline, e := os.ReadFile("../migrationsrp/" + PrivilegeMigration)
	require.NoError(t, e)
	require.Equal(t, PrivilegeMigrationSHA256, MigrationFingerprint(baseline))

	upgrade, e := os.ReadFile("../migrationsrp/" + CreditKindPrivilegeMigration)
	require.NoError(t, e)

	grantsByMigration := map[string]string{
		PrivilegeMigration:           string(baseline),
		CreditKindPrivilegeMigration: string(upgrade),
	}

	seen := map[string]bool{}
	for _, p := range PrivilegeRegistry() {
		require.False(t, seen[p.Table])
		seen[p.Table] = true
		require.Equal(t, "bnpl", p.Schema)
		require.NotEmpty(t, p.Description)
		require.Contains(t, []string{PrivilegeMigration, CreditKindPrivilegeMigration}, p.Migration)
		for _, op := range p.Operations {
			require.Contains(t, []Operation{SelectOperation, InsertOperation, UpdateOperation}, op)
		}

		migrationSQL, ok := grantsByMigration[p.Migration]
		require.True(t, ok, "unknown privilege migration %s", p.Migration)

		if len(p.UpdateColumns) > 0 {
			require.NotContains(t, p.Operations, UpdateOperation)
			// Column grants for a baseline table may be completed by a later
			// reviewed upgrade (for example checkout_sessions.credit_kind).
			combined := migrationSQL + "\n" + string(upgrade)
			require.Truef(
				t,
				strings.Contains(
					combined,
					"GRANT UPDATE ("+strings.Join(p.UpdateColumns, ", ")+") ON bnpl."+p.Table+" TO bnpl;",
				) || grantColumnsCovered(combined, p.Table, p.UpdateColumns),
				"missing reviewed UPDATE column grant for bnpl.%s",
				p.Table,
			)
		}

		if p.Migration == CreditKindPrivilegeMigration {
			require.Contains(t, migrationSQL, "GRANT SELECT, INSERT ON bnpl."+p.Table+" TO bnpl;")
		}
	}
	require.Len(t, seen, 42)
	a := PrivilegeRegistry()
	a[0].Operations[0] = "DELETE"
	require.Equal(t, SelectOperation, PrivilegeRegistry()[0].Operations[0])
}

// grantColumnsCovered reports whether every required UPDATE column appears in
// one or more GRANT UPDATE (...) statements for the table. Additive grants
// across baseline + upgrade are accepted.
func grantColumnsCovered(sql, table string, required []string) bool {
	marker := ") ON bnpl." + table + " TO bnpl;"
	granted := map[string]bool{}
	for {
		end := strings.Index(sql, marker)
		if end < 0 {
			break
		}
		head := sql[:end]
		start := strings.LastIndex(head, "GRANT UPDATE (")
		if start < 0 {
			sql = sql[end+len(marker):]
			continue
		}
		cols := head[start+len("GRANT UPDATE ("):]
		for _, col := range strings.Split(cols, ", ") {
			granted[strings.TrimSpace(col)] = true
		}
		sql = sql[end+len(marker):]
	}
	for _, col := range required {
		if !granted[col] {
			return false
		}
	}
	return len(required) > 0
}
