package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres/migrationsrp"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/usecase/migrationsuc"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Database Database
	Gin      Gin
	Usecases Usecases
}

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

func (c *Config) ConnectionInfo() (dbName, host string, port int) {
	return c.Database.ConnectionInfo()
}

func (c *Config) NewSchemaRepo() repo.Schema {
	return c.Database.NewSchemaRepo()
}

func (c *Config) GetRolePassword(r repo.Role) (string, error) {
	return c.Database.GetRolePassword(r)
}

func (c *Config) SchemaInitializer(tx repo.Tx) repo.SchemaInitializer {
	return migrationsrp.NewInitializer(tx)
}

func (c *Config) NewMigrationsUseCase(
	p repo.Pool, r repo.Migrations,
) *migrationsuc.UseCase {
	return c.Usecases.Migrations.NewUseCase(p, r)
}

func (c *Config) String() string {
	marshaledConfig, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return ""
	}

	return string(marshaledConfig)
}
