package invariants

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryMatchesVersionedMigration(t *testing.T) {
	sql, e := os.ReadFile("../../../../../assets/db/migrations/" + Migration)
	require.NoError(t, e)
	sum := sha256.Sum256(sql)
	require.Equal(t, MigrationSHA256, "sha256:"+hex.EncodeToString(sum[:]))
	bodies := map[string]string{}
	for _, m := range regexp.MustCompile(`(?s)CREATE FUNCTION bnpl\.(\w+)\(\) RETURNS trigger\nLANGUAGE plpgsql SET search_path = pg_catalog, bnpl AS \$body\$(.*?)\$body\$;`).FindAllStringSubmatch(string(sql), -1) {
		sum := sha256.Sum256([]byte(m[2]))
		bodies[m[1]] = hex.EncodeToString(sum[:])
	}
	seen := map[string]bool{}
	for _, v := range Registry() {
		require.False(t, seen[v.Trigger])
		seen[v.Trigger] = true
		require.NotEmpty(t, v.Description)
		require.NotEmpty(t, v.Guidance)
		require.Equal(t, bodies[v.Function], v.FunctionSHA256)
		require.True(t, strings.Contains(string(sql), "TRIGGER "+v.Trigger+" "))
	}
	require.Len(t, seen, 8)
	copy := Registry()
	copy[0].Name = "corrupted"
	require.NotEqual(t, "corrupted", Registry()[0].Name)
}
