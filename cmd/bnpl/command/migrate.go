package command

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/config"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres/migrationsrp"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate a source database contents to a new empty database",
	Long: `Migrate a database contents to a change the format of schema
contents in order to match with the expected schema.
For this purpose, a configuration file path ia required.`,
	RunE: migrate,
	Args: cobra.NoArgs,
}

func migrate(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}

	p, err := c.ConnectionPool(ctx, repo.NormalRole)
	if err != nil {
		return fmt.Errorf("creating DB pool: %w", err)
	}
	defer func() {
		if err = p.Close(); err != nil {
			log.Printf("failed to close pool connection: %v", err)
		}
	}()

	migrationsRepo := migrationsrp.New()
	migrationsUC := c.NewMigrationsUseCase(p, migrationsRepo)
	err = migrationsUC.MigrateAll(ctx)
	if err != nil {
		return fmt.Errorf("migrate.MigrateAll: %w", err)
	}

	return nil
}

func init() {
	dbCmd.AddCommand(migrateCmd)
}
