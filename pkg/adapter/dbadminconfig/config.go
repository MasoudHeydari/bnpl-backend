// Package dbadminconfig loads only database administration settings. It must not
// import the Runtime config package or construct business dependencies.
package dbadminconfig

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/connection"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/schemarp"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"gopkg.in/yaml.v3"
)

// Config is the independently validated administration document. Owner has no secret.
type Config struct {
	Application string `yaml:"application"`
	Runtime     struct {
		Environment string `yaml:"environment"`
	} `yaml:"runtime"`
	Versions struct {
		Config   model.SemVer `yaml:"config"`
		Database model.SemVer `yaml:"database"`
	} `yaml:"versions"`
	Database struct {
		connection.Settings `yaml:",inline"`
		PGPassEntries       struct {
			Admin    model.Secret `yaml:"admin"`
			Migrator model.Secret `yaml:"migrator"`
		} `yaml:"pgpass_entries"`
	} `yaml:"database"`
	Provisioning struct {
		NormalPassword model.Secret `yaml:"normal_password"`
	} `yaml:"provisioning"`
	Migrations struct {
		FilesDir    string        `yaml:"files-dir"`
		LockTimeout time.Duration `yaml:"lock_timeout"`
	} `yaml:"migration"`
	Logging struct {
		Format string `yaml:"format"`
	} `yaml:"logging"`
}

// Load decodes no business configuration and never includes expanded values in errors.
func Load(path string) (*Config, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, fmt.Errorf("dbadmin config: cannot read file: %w", e)
	}
	var c Config
	decoder := yaml.NewDecoder(strings.NewReader(os.ExpandEnv(string(data))))
	decoder.KnownFields(true)
	if e = decoder.Decode(&c); e != nil {
		return nil, fmt.Errorf("dbadmin config: invalid YAML shape or value")
	}
	if e = c.Validate(); e != nil {
		return nil, e
	}
	return &c, nil
}

// Validate checks only administrative requirements, preserving version/environment gates.
func (c *Config) Validate() error {
	if c.Application != "dbadmin" {
		return fmt.Errorf("dbadmin config: application must be dbadmin")
	}
	if _, e := model.ParseRuntimeEnvironment(c.Runtime.Environment); e != nil {
		return fmt.Errorf("dbadmin config: invalid environment")
	}
	if e := validateBuildEnvironment(c.Runtime.Environment); e != nil {
		return e
	}
	if c.Versions.Config[0] != 0 || c.Versions.Config[1] > 3 || c.Versions.Config == (model.SemVer{}) || c.Versions.Database[0] != 0 || c.Versions.Database[1] > 2 || c.Versions.Database == (model.SemVer{}) {
		return fmt.Errorf("dbadmin config: incompatible versions")
	}
	if c.Database.Host == "" || c.Database.Name == "" || c.Database.Port < 1 || c.Database.Port > 65535 || c.Database.SSLMode == "" || c.Database.MaxConnections < 1 || c.Database.ConnectionTimeout <= 0 {
		return fmt.Errorf("dbadmin config: invalid database transport settings")
	}
	if c.Database.QueryLogging.Enabled {
		return fmt.Errorf("dbadmin config: administrative query logging must be disabled")
	}
	if c.Migrations.FilesDir == "" || c.Migrations.LockTimeout < 0 {
		return fmt.Errorf("dbadmin config: invalid migration settings")
	}
	if c.Logging.Format != "json" && c.Logging.Format != "text" {
		return fmt.Errorf("dbadmin config: logging.format must be json or text")
	}
	return nil
}

// GetRolePassword resolves only the selected administrative credential. Provisioning
// of the normal password is explicit; it never establishes a normal connection.
func (c *Config) GetRolePassword(r repo.Role) (string, error) {
	if r == repo.NormalRole {
		if c.Provisioning.NormalPassword.IsEmpty() {
			return "", fmt.Errorf("dbadmin: provisioning.normal_password required for role provisioning")
		}
		return c.Provisioning.NormalPassword.Reveal(), nil
	}
	var line string
	switch r {
	case repo.AdminRole:
		line = c.Database.PGPassEntries.Admin.Reveal()
	case repo.MigratorRole:
		line = c.Database.PGPassEntries.Migrator.Reveal()
	default:
		return "", fmt.Errorf("dbadmin: role has no login credential")
	}
	prefix := fmt.Sprintf("%s:%d:%s:%s:", c.Database.Host, c.Database.Port, c.Database.Name, r)
	if !strings.HasPrefix(line, prefix) || len(line) == len(prefix) {
		return "", fmt.Errorf("dbadmin: missing/mismatched %s pgpass entry", r)
	}
	return strings.TrimPrefix(line, prefix), nil
}

// ConnectionPool opens only admin or migrator. Every physical migrator session
// explicitly assumes NOLOGIN owner before use, including nontransactional DDL.
func (c *Config) ConnectionPool(ctx context.Context, r repo.Role, _ corelog.Logger) (repo.Pool, error) {
	if r != repo.AdminRole && r != repo.MigratorRole {
		return nil, fmt.Errorf("dbadmin: connection role forbidden")
	}
	password, e := c.GetRolePassword(r)
	if e != nil {
		return nil, e
	}
	u := url.URL{Scheme: "postgresql", Host: fmt.Sprintf("%s:%d", c.Database.Host, c.Database.Port), Path: c.Database.Name, User: url.UserPassword(string(r), password)}
	q := u.Query()
	q.Set("sslmode", c.Database.SSLMode)
	u.RawQuery = q.Encode()
	settings := c.Database.Settings
	settings.QueryLogging = connection.QueryLogging{} // Never trace credential-bearing administrative SQL.
	cfg, e := settings.Config(u.String(), nil)
	if e != nil {
		return nil, fmt.Errorf("dbadmin: invalid PostgreSQL connection settings")
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if r == repo.MigratorRole {
			if _, e := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{string(repo.OwnerRole)}.Sanitize()); e != nil {
				return fmt.Errorf("dbadmin: migrator cannot assume owner")
			}
		}
		_, e := conn.Exec(ctx, "SET search_path = bnpl, pg_catalog")
		return e
	}
	return postgres.NewPool(ctx, cfg)
}

// ConnectionInfo returns non-secret target identity.
func (c *Config) ConnectionInfo() (string, string, int) {
	return c.Database.Name, c.Database.Host, c.Database.Port
}

// NewSchemaRepo reuses the existing schema adapter.
func (c *Config) NewSchemaRepo() repo.Schema { return schemarp.New() }

// SchemaInitializer reuses the frozen baseline and build-specific seed initializer.
func (c *Config) SchemaInitializer(tx repo.Tx) repo.SchemaInitializer {
	return migrationsrp.NewInitializer(tx)
}
