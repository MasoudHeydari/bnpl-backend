package config

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/schemarp"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Database defines PostgreSQL connection and pool settings.
// The pgpass entry fields must contain full `host:port:database:user:password`
// lines so role passwords can be resolved deterministically at runtime.
type Database struct {
	Host                  string        `yaml:"host"`
	Port                  int           `yaml:"port"`
	Name                  string        `yaml:"name"`
	MaxConnections        int32         `yaml:"max_connections"`
	MinConnections        int32         `yaml:"min_connections"`
	MaxConnectionLifeTime time.Duration `yaml:"max_connection_life_time"`
	MaxConnectionIdleTime time.Duration `yaml:"max_connection_idle_time"`
	HealthCheckPeriod     time.Duration `yaml:"health_check_period"`
	ConnectionTimeout     time.Duration `yaml:"connection_timeout"`
	SSLMode               string        `yaml:"ssl_mode"`
	PGPassEntries         struct {
		Admin  model.Secret `yaml:"admin"`
		Normal model.Secret `yaml:"normal"`
	} `yaml:"pgpass_entries"`
}

// ConnectionPool builds a PostgreSQL connection pool for role using the
// adapter-layer postgres implementation. The returned pool validates
// connectivity before being returned and must be closed by the caller.
// Pool instances are safe for concurrent use after construction.
func (d Database) ConnectionPool(ctx context.Context, r repo.Role) (repo.Pool, error) {
	u, err := d.ConnectionURL(r)
	if err != nil {
		return nil, fmt.Errorf("ConnectionPool: %w", err)
	}
	p, err := postgres.NewPool(ctx, d.getConfig(u))
	if err != nil {
		return nil, fmt.Errorf("ConnectionPool: failed to create new connection pool %v", err)
	}
	return p, nil
}

// ConnectionURL builds a PostgreSQL connection URL for the given database role.
// Credentials are resolved from the configured pgpass entries rather than
// being stored directly in the generated URL configuration. An error is
// returned if no pgpass entry matches the requested role.
// The resulting URL includes the configured sslmode parameter and is suitable
// for creating pgx connection pools and direct PostgreSQL connections.
func (d Database) ConnectionURL(r repo.Role) (string, error) {
	pass, err := d.GetRolePassword(r)
	if err != nil {
		return "", fmt.Errorf("ConnectionURL: resolve role password: %w", err)
	}

	u := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(string(r), pass),
		Host:   fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:   d.Name,
	}

	q := u.Query()
	q.Set("sslmode", d.SSLMode)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// ConnectionInfo returns the configured database name, host, and port.
func (d Database) ConnectionInfo() (dbName, host string, port int) {
	return d.Name, d.Host, d.Port
}

// getConfig parses connURL into a pgx pool configuration and applies the
// repository's pool and timeout settings.
// The function terminates the process through log.Fatal when connURL cannot be
// parsed, so callers are expected to pass a URL that was already constructed by
// ConnectionURL.
func (d Database) getConfig(connUrl string) *pgxpool.Config {
	pgConfig, err := pgxpool.ParseConfig(connUrl)
	if err != nil {
		log.Fatal("Failed to create a config, error", err)
	}

	pgConfig.MaxConns = d.MaxConnections
	pgConfig.MinConns = d.MinConnections
	pgConfig.MaxConnLifetime = d.MaxConnectionLifeTime
	pgConfig.MaxConnIdleTime = d.MaxConnectionIdleTime
	pgConfig.HealthCheckPeriod = d.HealthCheckPeriod
	pgConfig.ConnConfig.ConnectTimeout = d.ConnectionTimeout

	return pgConfig
}

// NewSchemaRepo returns the PostgreSQL-backed schema repository adapter.
// The repository is stateless and safe to reuse across requests.
func (d Database) NewSchemaRepo() repo.Schema {
	return schemarp.New()
}

// GetRolePassword extracts the password for role from the configured pgpass
// entry lines. The function expects exact host, port, database, and role
// matches and returns an error when no entry matches.
func (d Database) GetRolePassword(r repo.Role) (string, error) {
	prfx := fmt.Sprintf("%s:%d:%s:%s:", d.Host, d.Port, d.Name, r)
	passLines := []string{
		d.PGPassEntries.Admin.Reveal(),
		d.PGPassEntries.Normal.Reveal(),
	}

	var pass string
	for _, line := range passLines {
		if line == "" || line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, prfx) {
			pass = line[len(prfx):]
			break
		}
	}
	if pass == "" {
		return "", fmt.Errorf("no matching password line")
	}

	return pass, nil
}
