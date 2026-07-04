package config

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/connection"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Database defines PostgreSQL connection and pool settings.
// The pgpass entry fields must contain full `host:port:database:user:password`
// lines so role passwords can be resolved deterministically at runtime.
type Database struct {
	BusinessTimezone string `yaml:"-"`
	// Host is the PostgreSQL server hostname or address.
	Host string `yaml:"host"`

	// Port is the PostgreSQL server port.
	Port int `yaml:"port"`

	// Name is the PostgreSQL database name.
	Name string `yaml:"name"`

	// MaxConnections configures the maximum number of open pool connections.
	MaxConnections int32 `yaml:"max_connections"`

	// MinConnections configures the minimum number of idle pool connections.
	MinConnections int32 `yaml:"min_connections"`

	// MaxConnectionLifeTime configures how long a connection may remain open.
	MaxConnectionLifeTime time.Duration `yaml:"max_connection_life_time"`

	// MaxConnectionIdleTime configures how long a connection may remain idle.
	MaxConnectionIdleTime time.Duration `yaml:"max_connection_idle_time"`

	// HealthCheckPeriod configures how often idle connections are health-checked.
	HealthCheckPeriod time.Duration `yaml:"health_check_period"`

	// ConnectionTimeout configures the timeout for establishing a new connection.
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`

	// SSLMode configures the PostgreSQL SSL mode (for example disable or require).
	SSLMode string `yaml:"ssl_mode"`

	// QueryLogging configures diagnostic PostgreSQL query logging.
	QueryLogging QueryLogging `yaml:"query_logging"`

	// PGPassEntries contains full pgpass lines used to resolve role passwords.
	PGPassEntries struct {
		// Normal is the pgpass line for the normal application database role.
		Normal model.Secret `yaml:"normal"`
	} `yaml:"pgpass_entries"`
}

// QueryLogging controls diagnostic PostgreSQL query logging.
//
// IncludeArguments renders positional query arguments into a logging-only SQL
// representation. Query execution remains parameterized by pgx.
type QueryLogging struct {
	// Enabled controls whether PostgreSQL query logging is active.
	Enabled bool `yaml:"enabled"`

	// IncludeArguments controls whether positional query arguments are rendered
	// into the logging-only SQL representation.
	IncludeArguments bool `yaml:"include_arguments"`
}

// ConnectionPool builds only a normal-role PostgreSQL pool using the
// adapter-layer postgres implementation. The returned pool validates
// connectivity before being returned and must be closed by the caller.
// Pool instances are safe for concurrent use after construction.
func (d Database) ConnectionPool(
	ctx context.Context,
	r repo.Role,
	queryLogger corelog.Logger,
) (repo.Pool, error) {
	const op = "config.Database.ConnectionPool"

	u, err := d.ConnectionURL(r)
	if err != nil {
		return nil, fmt.Errorf("%s: ConnectionPool: %w", op, err)
	}

	pgConfig, err := d.getConfig(u, queryLogger)
	if err != nil {
		return nil, fmt.Errorf("%s: ConnectionPool: parse pool config: %w", op, err)
	}

	p, err := postgres.NewPool(ctx, pgConfig)
	if err != nil {
		return nil, fmt.Errorf("%s: ConnectionPool: failed to create new connection pool: %w", op, err)
	}

	return p, nil
}

// ConnectionURL builds a PostgreSQL URL only for NormalRole; privileged roles
// are rejected before credential resolution.
// Credentials are resolved from the configured pgpass entries rather than
// being stored directly in the generated URL configuration. An error is
// returned if no pgpass entry matches the requested role.
// The resulting URL includes the configured sslmode parameter and is suitable
// for creating pgx connection pools and direct PostgreSQL connections.
func (d Database) ConnectionURL(r repo.Role) (string, error) {
	const op = "config.Database.ConnectionURL"

	pass, err := d.getRolePassword(r)
	if err != nil {
		return "", fmt.Errorf("%s: ConnectionURL: resolve role password: %w", op, err)
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

// connectionInfo returns the configured database name, host, and port.
func (d Database) connectionInfo() (dbName, host string, port int) {
	return d.Name, d.Host, d.Port
}

// getConfig adapts Runtime transport settings to the shared connection adapter.
func (d Database) getConfig(url string, logger corelog.Logger) (*pgxpool.Config, error) {
	return (connection.Settings{Host: d.Host, Port: d.Port, Name: d.Name, SSLMode: d.SSLMode, MaxConnections: d.MaxConnections, MinConnections: d.MinConnections, MaxConnectionLifeTime: d.MaxConnectionLifeTime, MaxConnectionIdleTime: d.MaxConnectionIdleTime, HealthCheckPeriod: d.HealthCheckPeriod, ConnectionTimeout: d.ConnectionTimeout, BusinessTimezone: d.BusinessTimezone, QueryLogging: connection.QueryLogging{Enabled: d.QueryLogging.Enabled, IncludeArguments: d.QueryLogging.IncludeArguments}}).Config(url, logger)
}

// getRolePassword extracts the password for role from the configured pgpass
// entry lines. The function expects exact host, port, database, and role
// matches and returns an error when no entry matches.
func (d Database) getRolePassword(r repo.Role) (string, error) {
	const op = "config.Database.getRolePassword"
	if r != repo.NormalRole {
		return "", fmt.Errorf("%s: Runtime only supports the normal database role", op)
	}

	prfx := fmt.Sprintf(
		"%s:%d:%s:%s:",
		d.Host,
		d.Port,
		d.Name,
		r,
	)
	passLines := []string{
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
		return "", fmt.Errorf("%s: no matching password line", op)
	}

	return pass, nil
}
