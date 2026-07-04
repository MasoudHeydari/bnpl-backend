package config

import (
	"strings"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRejectsPrivilegedConfigAndCredentials(t *testing.T) {
	// ConnectionURL rejects privileged roles for both build profiles. Use the
	// shared Validate()-clean fixture so Production binaries are not skipped.
	cfg := validConfig()
	require.NoError(t, cfg.Validate())

	for _, r := range []repo.Role{repo.AdminRole, repo.OwnerRole, repo.MigratorRole} {
		_, e := cfg.Database.ConnectionURL(r)
		require.Error(t, e)
	}

	// Load rejects privileged application / credential shape before retaining
	// secrets. These corruptions fail closed prior to provider-profile checks,
	// so the development minimal YAML remains a valid base under Production.
	valid := validVersionsYAML + validMinimalValidationYAML
	for _, bad := range []string{
		strings.Replace(valid, "application: runtime", "application: dbadmin", 1),
		strings.Replace(valid, "pgpass_entries:", "pgpass_entries:\n    admin: secret-do-not-print", 1),
		strings.Replace(valid, "port: 5432", "port: secret-do-not-print", 1),
	} {
		_, e := Load(writeConfigFile(t, bad))
		require.Error(t, e)
		require.NotContains(t, e.Error(), "secret-do-not-print")
	}
}
