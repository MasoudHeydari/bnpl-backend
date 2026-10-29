package invariants

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrivilegeRegistryMatchesInitialSchema(t *testing.T) {
	data, e := os.ReadFile("../migrationsrp/" + PrivilegeMigration)
	require.NoError(t, e)
	require.Equal(t, PrivilegeMigrationSHA256, MigrationFingerprint(data))
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
			require.Contains(t, string(data), "GRANT UPDATE ("+strings.Join(p.UpdateColumns, ", ")+") ON bnpl."+p.Table+" TO bnpl;")
		}
	}
	require.Len(t, seen, 39)
	a := PrivilegeRegistry()
	a[0].Operations[0] = "DELETE"
	require.Equal(t, SelectOperation, PrivilegeRegistry()[0].Operations[0])
}
