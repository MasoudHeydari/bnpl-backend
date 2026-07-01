package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validVersionsYAML = `
versions:
  database: 0.2.0
  config: 0.2.0
`

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoad_ValidMinimalConfig(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfigFile(t, validVersionsYAML))
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, Version, cfg.Versions.Config)
	assert.Equal(t, DatabaseVersion, cfg.Versions.Database)
}

func TestLoad_FullConfigSections(t *testing.T) {
	t.Parallel()

	content := validVersionsYAML + `
database:
  host: localhost
  port: 5432
  name: bnpl
gin:
  host: 0.0.0.0
  port: 8080
  logger: true
  recovery: true
usecases:
  health:
    enabled: true
  migration:
    files-dir: ./migrations
  outbox_publisher:
    worker_id: worker-a
    batch_size: 50
infrastructure:
  shutdown:
    timeout: 45s
  health:
    checks:
      - database
`

	cfg, err := Load(writeConfigFile(t, content))
	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "bnpl", cfg.Database.Name)
	assert.Equal(t, "0.0.0.0", cfg.Gin.Host)
	assert.Equal(t, 8080, cfg.Gin.Port)
	assert.True(t, cfg.Gin.Logger)
	assert.True(t, cfg.Gin.Recovery)
	assert.True(t, cfg.Usecases.Health.Enabled)
	assert.Equal(t, "worker-a", cfg.Usecases.OutboxPublisher.WorkerID)
	assert.Equal(t, 50, cfg.Usecases.OutboxPublisher.BatchSize)
}

func TestLoad_ExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("TEST_DB_HOST", "postgres.local")
	t.Setenv("TEST_GIN_PORT", "9090")

	content := validVersionsYAML + `
database:
  host: ${TEST_DB_HOST}
gin:
  port: ${TEST_GIN_PORT}
`

	cfg, err := Load(writeConfigFile(t, content))
	require.NoError(t, err)
	assert.Equal(t, "postgres.local", cfg.Database.Host)
	assert.Equal(t, 9090, cfg.Gin.Port)
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, ":\n  bad: ["))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshalling yaml")
}

func TestLoad_EmptyFile(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, ""))
	require.Error(t, err)
}

func TestLoad_ScalarRootDocument(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, "not-a-mapping"))
	require.Error(t, err)
}

func TestLoad_UnsupportedConfigVersion(t *testing.T) {
	t.Parallel()

	content := `
versions:
  database: 0.2.0
  config: 0.9.0
`

	_, err := Load(writeConfigFile(t, content))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating versions")
	assert.Contains(t, err.Error(), "unsupported minor version")
}

func TestLoad_MissingVersions(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, "database:\n  host: localhost\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating versions")
}

func TestConfig_ValidateVersions(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Versions: Versions{
			Config:   Version,
			Database: DatabaseVersion,
		},
	}

	require.NoError(t, cfg.ValidateVersions())
}

func TestConfig_String_RedactsSecrets(t *testing.T) {
	t.Parallel()

	content := validVersionsYAML + `
database:
  pgpass_entries:
    admin: localhost:5432:bnpl:admin:secret-value
    normal: localhost:5432:bnpl:bnpl:secret-value
`
	loaded, err := Load(writeConfigFile(t, content))
	require.NoError(t, err)

	out := loaded.String()
	require.NotEmpty(t, out)
	assert.NotContains(t, out, "secret-value")
	assert.Contains(t, out, "REDACTED")
}
