package config

import (
	"strings"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRejectsPrivilegedConfigAndCredentials(t *testing.T) {
	valid := validVersionsYAML + validMinimalValidationYAML
	c, e := Load(writeConfigFile(t, valid))
	require.NoError(t, e)
	for _, r := range []repo.Role{repo.AdminRole, repo.OwnerRole, repo.MigratorRole} {
		_, e = c.Database.ConnectionURL(r)
		require.Error(t, e)
	}
	for _, bad := range []string{strings.Replace(valid, "application: runtime", "application: dbadmin", 1), strings.Replace(valid, "pgpass_entries:", "pgpass_entries:\n    admin: secret-do-not-print", 1), strings.Replace(valid, "port: 5432", "port: secret-do-not-print", 1)} {
		_, e = Load(writeConfigFile(t, bad))
		require.Error(t, e)
		require.NotContains(t, e.Error(), "secret-do-not-print")
	}
}
