package config

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Config file schema version supported by this binary.
const (
	Major = 0
	Minor = 3
	Patch = 0
)

// Version is the semantic version of the configuration document schema
// supported by this binary.
var Version = model.SemVer{Major, Minor, Patch}

// Database schema version supported by this binary.
const (
	DatabaseMajor = 0
	DatabaseMinor = 2
	DatabasePatch = 0
)

// DatabaseVersion is the semantic version of the database schema supported by
// this binary for non-migration operations.
var DatabaseVersion = model.SemVer{DatabaseMajor, DatabaseMinor, DatabasePatch}

// Versions contains the configuration file and database schema versions
// declared in YAML. Although migration tooling may support a range of
// versions, each binary only supports the versions defined by Version
// and DatabaseVersion for normal runtime operations.
type Versions struct {
	// Database is the declared database schema semantic version.
	Database model.SemVer `yaml:"database"`

	// Config is the declared configuration file schema semantic version.
	Config model.SemVer `yaml:"config"`
}

// Validate checks that declared configuration and database schema versions
// are supported by this binary.
func (v Versions) Validate() error {
	if err := v.ValidateConfig(); err != nil {
		return err
	}

	return v.ValidateDatabase()
}

// ValidateConfig checks the declared configuration file version.
func (v Versions) ValidateConfig() error {
	return validateSemVer("versions.config", v.Config, Version)
}

// ValidateDatabase checks the declared database schema version.
func (v Versions) ValidateDatabase() error {
	return validateSemVer("versions.database", v.Database, DatabaseVersion)
}

// validateSemVer reports whether declared is supported by supported.
// The declared major version must match and the declared minor version must
// not be newer than supported.
func validateSemVer(field string, declared, supported model.SemVer) error {
	if declared == (model.SemVer{}) {
		return fmt.Errorf("%s: required", field)
	}

	if declared[0] != supported[0] {
		return fmt.Errorf(
			"%s: incompatible major version: %d",
			field,
			declared[0],
		)
	}

	if declared[1] > supported[1] {
		return fmt.Errorf(
			"%s: unsupported minor version: %d",
			field,
			declared[1],
		)
	}

	return nil
}
