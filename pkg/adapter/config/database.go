package config

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres/schemarp"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
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
	PassDir               string        `yaml:"pass-dir"`
}

func (d Database) ConnectionPool(ctx context.Context, r repo.Role) (repo.Pool, error) {
	path := filepath.Join(d.PassDir, ".pgpass")
	u, err := d.ConnectionURL(r, path)
	if err != nil {
		return nil, fmt.Errorf("using %q pass-file: %w", path, err)
	}
	p, err := postgres.NewPool(ctx, d.getConfig(u))
	if err != nil {
		return nil, fmt.Errorf("failed to connect with %q: %v", path, err)
	}
	return p, nil
}

func (d Database) ConnectionURL(r repo.Role, path string) (string, error) {
	passLines, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading pass-dir: %w", err)
	}
	prfx := fmt.Sprintf("%s:%d:%s:%s:", d.Host, d.Port, d.Name, r)
	var pass string
	for _, line := range strings.Split(string(passLines), "\n") {
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
	u := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(string(r), pass),
		Host:   fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:   d.Name,
	}
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
