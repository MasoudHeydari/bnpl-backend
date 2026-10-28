package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func testDatabase(t *testing.T) Database {
	t.Helper()

	return Database{
		Host:    "db.example.com",
		Port:    5432,
		Name:    "bnpl",
		SSLMode: "disable",
		PGPassEntries: struct {
			Normal model.Secret `yaml:"normal"`
		}{
			Normal: model.NewSecret("db.example.com:5432:bnpl:bnpl:normal-secret"),
		},
	}
}

func TestDatabase_getRolePassword_AdminRole(t *testing.T) {
	_, err := testDatabase(t).getRolePassword(repo.AdminRole)
	require.Error(t, err)
}

func TestDatabase_getRolePassword_NormalRole(t *testing.T) {
	t.Parallel()

	pass, err := testDatabase(t).getRolePassword(repo.NormalRole)
	require.NoError(t, err)
	assert.Equal(t, "normal-secret", pass)
}

func TestDatabase_getRolePassword_SkipsCommentEntry(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	db.PGPassEntries.Normal = model.NewSecret("db.example.com:5432:bnpl:bnpl:normal-secret")

	pass, err := db.getRolePassword(repo.NormalRole)
	require.NoError(t, err)
	assert.Equal(t, "normal-secret", pass)
}

func TestDatabase_getRolePassword_NoMatchingEntry(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	db.PGPassEntries.Normal = model.NewSecret("other-host:5432:other:bnpl:secret")

	_, err := db.getRolePassword(repo.NormalRole)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no matching password line")
}

func TestDatabase_ConnectionURL_NormalRole(t *testing.T) {
	t.Parallel()

	connURL, err := testDatabase(t).ConnectionURL(repo.NormalRole)
	require.NoError(t, err)
	assert.Equal(
		t,
		"postgresql://bnpl:normal-secret@db.example.com:5432/bnpl?sslmode=disable",
		connURL,
	)
}

func TestDatabase_ConnectionURL_AdminRole(t *testing.T) {
	_, err := testDatabase(t).ConnectionURL(repo.AdminRole)
	require.Error(t, err)
}

func TestDatabase_ConnectionURL_MissingPassword(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	db.PGPassEntries.Normal = model.NewSecret("")

	_, err := db.ConnectionURL(repo.NormalRole)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ConnectionURL: resolve role password")
}

func TestDatabase_getConfig_QueryLoggingInstallsTracer(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	db.QueryLogging = QueryLogging{
		Enabled:          true,
		IncludeArguments: true,
	}

	connURL, err := db.ConnectionURL(repo.NormalRole)
	require.NoError(t, err)

	logger := lognoop.New()
	cfg, err := db.getConfig(connURL, logger)
	require.NoError(t, err)
	require.NotNil(t, cfg.ConnConfig.Tracer)
}

func TestDatabase_getConfig_QueryLoggingRequiresLogger(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	db.QueryLogging.Enabled = true

	connURL, err := db.ConnectionURL(repo.NormalRole)
	require.NoError(t, err)

	_, err = db.getConfig(connURL, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "query logger is required")
}
