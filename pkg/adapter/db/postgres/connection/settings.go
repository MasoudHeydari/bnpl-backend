// Package connection shares credential-free PostgreSQL transport settings.
package connection

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/querylog"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// Settings contains reusable transport settings without role credentials.
type Settings struct {
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

// Config parses connURL into a pgx pool configuration and applies the
// repository's pool and timeout settings.
func (d Settings) Config(
	connURL string,
	queryLogger corelog.Logger,
) (*pgxpool.Config, error) {
	const op = "config.Database.getConfig"

	pgConfig, err := pgxpool.ParseConfig(connURL)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid PostgreSQL pool configuration", op)
	}

	if d.BusinessTimezone != "" {
		pgConfig.ConnConfig.RuntimeParams["timezone"] = d.BusinessTimezone
	}

	pgConfig.MaxConns = d.MaxConnections
	pgConfig.MinConns = d.MinConnections
	pgConfig.MaxConnLifetime = d.MaxConnectionLifeTime
	pgConfig.MaxConnIdleTime = d.MaxConnectionIdleTime
	if d.HealthCheckPeriod > 0 {
		pgConfig.HealthCheckPeriod = d.HealthCheckPeriod
	}
	pgConfig.ConnConfig.ConnectTimeout = d.ConnectionTimeout

	if d.QueryLogging.Enabled {
		if queryLogger == nil {
			return nil, fmt.Errorf("%s: query logger is required when database query logging is enabled", op)
		}

		pgConfig.ConnConfig.Tracer = querylog.New(
			queryLogger,
			d.QueryLogging.IncludeArguments,
		)
	}

	return pgConfig, nil
}
