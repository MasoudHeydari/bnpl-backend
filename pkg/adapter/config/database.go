package config

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres/schemarp"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

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
		Admin  string `yaml:"admin"`
		Normal string `yaml:"normal"`
	} `yaml:"pgpass_entries"`
}

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

func (d Database) ConnectionURL(r repo.Role) (string, error) {
	pass, err := d.GetRolePassword(r)
	if err != nil {
		return "", fmt.Errorf("ConnectionURL: %w", err)
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

func (d Database) ConnectionInfo() (dbName, host string, port int) {
	return d.Name, d.Host, d.Port
}

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

func (d Database) NewSchemaRepo() repo.Schema {
	return schemarp.New()
}

func (d Database) GetRolePassword(r repo.Role) (string, error) {
	prfx := fmt.Sprintf("%s:%d:%s:%s:", d.Host, d.Port, d.Name, r)
	passLines := []string{d.PGPassEntries.Admin, d.PGPassEntries.Normal}

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
