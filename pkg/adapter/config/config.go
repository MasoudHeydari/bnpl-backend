// Package config loads runtime configuration and exposes constructors that
// adapt configuration values into infrastructure components and use cases.
// It belongs to the adapter layer. The package depends on YAML decoding,
// PostgreSQL adapters, and migration use cases, and acts as the boundary
// between external configuration files and the rest of the application.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	"gopkg.in/yaml.v3"
)

// Config is the root runtime configuration document.
// It aggregates adapter and application settings loaded from YAML and
// environment variable expansion. Callers typically load it once at startup
// and treat it as immutable afterwards.
type Config struct {
	Database       Database
	Gin            Gin
	Usecases       Usecases
	Infrastructure Infrastructure
}

// Load reads a YAML configuration file from path, expands ${VAR} placeholders
// from the current process environment, and decodes the result into Config.
// It returns an error when the file cannot be read, the YAML document shape is
// unexpected, or decoding fails. The function is concurrency-safe because it
// only works with local state and process environment reads.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	n := &yaml.Node{}
	expandedEnv := os.ExpandEnv(string(data))
	if err = yaml.Unmarshal([]byte(expandedEnv), n); err != nil {
		return nil, fmt.Errorf("unmarshalling yaml: %w", err)
	}
	if l := len(n.Content); l != 1 {
		return nil, fmt.Errorf("found %d children nodes, instead of 1 mapping child", l)
	}
	c := &Config{}
	if err = n.Decode(c); err != nil {
		return nil, fmt.Errorf("decoding yaml node: %w", err)
	}
	return c, nil
}

// ConnectionPool builds a database connection pool for role.
// The returned pool owns live network connections and must be closed by the
// caller. Pool creation may perform connectivity checks and therefore has
// observable side effects on the target database.
func (c *Config) ConnectionPool(
	ctx context.Context, r repo.Role,
) (repo.Pool, error) {
	p, err := c.Database.ConnectionPool(ctx, r)
	if err != nil {
		return nil, fmt.Errorf(
			"%#v.ConnectionPool: %w", c.Database, err,
		)
	}
	return p, nil
}

// ConnectionInfo returns the configured database name, host, and port.
// It is a pure accessor and has no side effects.
func (c *Config) ConnectionInfo() (dbName, host string, port int) {
	return c.Database.connectionInfo()
}

// NewSchemaRepo constructs the schema repository adapter configured for this
// process. The returned repository is stateless and safe to reuse.
func (c *Config) NewSchemaRepo() repo.Schema {
	return c.Database.newSchemaRepo()
}

// GetRolePassword resolves the configured password for role from the pgpass
// entries embedded in the configuration. It returns an error when no matching
// entry exists.
func (c *Config) GetRolePassword(r repo.Role) (string, error) {
	return c.Database.getRolePassword(r)
}

// SchemaInitializer creates a schema initializer bound to tx.
// The initializer uses the supplied transaction exclusively; callers must not
// use it after the transaction has been committed or rolled back.
func (c *Config) SchemaInitializer(tx repo.Tx) repo.SchemaInitializer {
	return migrationsrp.NewInitializer(tx)
}

// NewMigrationsUseCase constructs the migration use case configured by this
// repository. The returned use case reuses the caller-provided pool and
// repository and does not take ownership beyond its own method calls.
func (c *Config) NewMigrationsUseCase(
	p repo.Pool, r repo.Migrations,
) *migrationsuc.UseCase {
	return c.Usecases.Migrations.newUseCase(p, r)
}

func (c *Config) NewGinEngine(provider trace.Provider) *gin.Engine {
	return c.Gin.newEngine(provider)
}

func (c *Config) GetGinListeningAddress() string {
	return c.Gin.getListeningAddress()
}

func (c *Config) NewTraceInfrastructure() (trace.Provider, error) {
	return c.Infrastructure.Trace.newTraceInfrastructure()
}

func (c *Config) NewSystemClockInfrastructure() (clock.Clock, error) {
	return c.Infrastructure.Clock.newSystemClockInfrastructure()
}

// String returns a JSON representation of c Config struct with secrets redacted.
func (c *Config) String() string {
	marshaledConfig, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return ""
	}

	return string(marshaledConfig)
}
