package invariants

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryMatchesInitialSchema(t *testing.T) {
	sql, e := os.ReadFile("../migrationsrp/" + Migration)
	require.NoError(t, e)
	sum := sha256.Sum256(sql)
	require.Equal(t, MigrationSHA256, "sha256:"+hex.EncodeToString(sum[:]))
	bodies := map[string]string{}
	const functionPattern = `(?is)CREATE\s+FUNCTION\s+bnpl\.(\w+)\s*\(\s*\)\s+RETURNS\s+trigger\s+` +
		`LANGUAGE\s+plpgsql\s+SET\s+search_path\s*=\s*pg_catalog,\s*bnpl\s+AS\s+` +
		`\$body\$(.*?)\$body\$;`
	for _, m := range regexp.MustCompile(functionPattern).FindAllStringSubmatch(string(sql), -1) {
		sum := sha256.Sum256([]byte(m[2]))
		bodies[m[1]] = hex.EncodeToString(sum[:])
	}
	triggers := map[string]bool{}
	const triggerPattern = `(?i)CREATE\s+(?:CONSTRAINT\s+)?TRIGGER\s+(\w+)\b`
	for _, m := range regexp.MustCompile(triggerPattern).FindAllStringSubmatch(string(sql), -1) {
		triggers[m[1]] = true
	}
	seen := map[string]bool{}
	for _, v := range Registry() {
		require.False(t, seen[v.Trigger])
		seen[v.Trigger] = true
		require.NotEmpty(t, v.Description)
		require.NotEmpty(t, v.Guidance)
		require.Equal(t, bodies[v.Function], v.FunctionSHA256)
		require.True(t, triggers[v.Trigger])
	}
	require.Len(t, seen, 8)
	copy := Registry()
	copy[0].Name = "corrupted"
	require.NotEqual(t, "corrupted", Registry()[0].Name)
}
