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

	migrationSQL := string(baseline)

	seen := map[string]bool{}
	for _, p := range PrivilegeRegistry() {
		require.False(t, seen[p.Table])
		seen[p.Table] = true
		require.Equal(t, "bnpl", p.Schema)
		require.NotEmpty(t, p.Description)
		require.Equal(t, PrivilegeMigration, p.Migration)
		for _, op := range p.Operations {
			require.Contains(t, []Operation{SelectOperation, InsertOperation, UpdateOperation}, op)
		}

		if len(p.UpdateColumns) > 0 {
			require.NotContains(t, p.Operations, UpdateOperation)
			require.Truef(
				t,
				strings.Contains(
					migrationSQL,
					"GRANT UPDATE ("+strings.Join(p.UpdateColumns, ", ")+") ON bnpl."+p.Table+" TO bnpl;",
				) || grantColumnsCovered(migrationSQL, p.Table, p.UpdateColumns),
				"missing reviewed UPDATE column grant for bnpl.%s",
				p.Table,
			)
		}

	}
	require.Len(t, seen, 43)
	a := PrivilegeRegistry()
	a[0].Operations[0] = "DELETE"
	require.Equal(t, SelectOperation, PrivilegeRegistry()[0].Operations[0])
}

// grantColumnsCovered reports whether every required UPDATE column appears in
// one or more GRANT UPDATE (...) statements for the table.
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
		for _, c := range strings.Split(cols, ",") {
			granted[strings.TrimSpace(c)] = true
		}
		sql = sql[end+len(marker):]
	}
	for _, c := range required {
		if !granted[c] {
			return false
		}
	}

	return true
}
