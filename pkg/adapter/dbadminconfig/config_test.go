package dbadminconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/require"
)

func configFile(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "admin.yaml")
	require.NoError(t, os.WriteFile(p, []byte(s), 0600))
	return p
}

const validConfigYAML = `application: dbadmin
runtime:
 environment: production
versions:
 config: 0.3.0
 database: 0.2.0
database:
 host: localhost
 port: 5432
 name: bnpl
 ssl_mode: disable
 max_connections: 2
 connection_timeout: 5s
 pgpass_entries:
  migrator: localhost:5432:bnpl:bnpl_migrator:test-secret-migrator
migration:
 files-dir: assets/db/migrations
logging:
 format: json
`

func TestAdminConfigHasNoBusinessDependenciesOrOwnerCredential(t *testing.T) {
	c, e := Load(configFile(t, validConfigYAML))
	require.NoError(t, e)
	p, e := c.GetRolePassword(repo.MigratorRole)
	require.NoError(t, e)
	require.Equal(t, "test-secret-migrator", p)
	for _, role := range []repo.Role{repo.OwnerRole, repo.AdminRole, repo.NormalRole} {
		_, e = c.GetRolePassword(role)
		require.Error(t, e)
		require.NotContains(t, e.Error(), "test-secret")
	}
	_, e = c.ConnectionPool(t.Context(), repo.NormalRole, nil)
	require.Error(t, e)
}
func TestAdminRejectsRuntimeAndSecretBearingInvalidValues(t *testing.T) {
	for _, s := range []string{strings.Replace(validConfigYAML, "application: dbadmin", "application: runtime", 1), validConfigYAML + "external:\n financial_core: unexpected\n", strings.Replace(validConfigYAML, "port: 5432", "port: private-password-here", 1), strings.Replace(validConfigYAML, "migrator: localhost", "owner: localhost", 1)} {
		_, e := Load(configFile(t, s))
		require.Error(t, e)
		require.NotContains(t, e.Error(), "private-password-here")
		require.NotContains(t, e.Error(), "test-secret-migrator")
	}
}
