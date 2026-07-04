package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestVersions_Validate_SupportedVersions(t *testing.T) {
	t.Parallel()

	v := Versions{
		Config:   Version,
		Database: DatabaseVersion,
	}

	require.NoError(t, v.Validate())
}

func TestVersions_Validate_OlderMinorVersion(t *testing.T) {
	t.Parallel()

	v := Versions{
		Config:   model.SemVer{0, 1, 5},
		Database: model.SemVer{0, 0, 9},
	}

	require.NoError(t, v.Validate())
}

func TestVersions_ValidateConfig_MissingConfigVersion(t *testing.T) {
	t.Parallel()

	v := Versions{
		Database: DatabaseVersion,
	}

	err := v.ValidateConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "versions.config: required")
}

func TestVersions_ValidateDatabase_MissingDatabaseVersion(t *testing.T) {
	t.Parallel()

	v := Versions{
		Config: Version,
	}

	err := v.ValidateDatabase()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "versions.database: required")
}

func TestVersions_ValidateConfig_IncompatibleMajorVersion(t *testing.T) {
	t.Parallel()

	v := Versions{
		Config:   model.SemVer{1, 0, 0},
		Database: DatabaseVersion,
	}

	err := v.ValidateConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "versions.config: incompatible major version: 1")
}

func TestVersions_ValidateDatabase_IncompatibleMajorVersion(t *testing.T) {
	t.Parallel()

	v := Versions{
		Config:   Version,
		Database: model.SemVer{2, 0, 0},
	}

	err := v.ValidateDatabase()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "versions.database: incompatible major version: 2")
}

func TestVersions_ValidateConfig_UnsupportedMinorVersion(t *testing.T) {
	t.Parallel()

	unsupportedMinor := Version[1] + 1
	v := Versions{
		Config:   model.SemVer{Version[0], unsupportedMinor, 0},
		Database: DatabaseVersion,
	}

	err := v.ValidateConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "versions.config: unsupported minor version")
}

func TestVersions_ValidateDatabase_UnsupportedMinorVersion(t *testing.T) {
	t.Parallel()

	unsupportedMinor := DatabaseVersion[1] + 1
	v := Versions{
		Config:   Version,
		Database: model.SemVer{DatabaseVersion[0], unsupportedMinor, 0},
	}

	err := v.ValidateDatabase()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "versions.database: unsupported minor version")
}

func TestVersions_Validate_ConfigFailurePrecedesDatabase(t *testing.T) {
	t.Parallel()

	v := Versions{
		Config:   model.SemVer{Version[0], Version[1] + 1, 0},
		Database: model.SemVer{9, 9, 9},
	}

	err := v.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "versions.config")
}
