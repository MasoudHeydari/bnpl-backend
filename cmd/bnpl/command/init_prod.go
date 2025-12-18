package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/config"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/usecase/migrationsuc"
)

var initProdCmd = &cobra.Command{
	Use:   "init-prod",
	Short: "Initialize database contents with production suitable data",
	Long: `Initialize database contents with production suitable data
for the database schema version which is specified in the configuration
file. The database connection information are also read from the config
file.`,
	RunE: initProd,
	Args: cobra.NoArgs,
}

func initProd(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}
	fmt.Printf("configs: %v\n", c)

	p, err := c.ConnectionPool(ctx, repo.NormalRole)
	if err != nil {
		return fmt.Errorf("creating DB pool: %w", err)
	}
	defer p.Close()

	muc := migrationsuc.NewInitDB(c)
	err = muc.InitProd(ctx)
	if err != nil {
		return fmt.Errorf("initializing DB with dev data: %w", err)
	}

	return nil
}

func init() {
	dbCmd.AddCommand(initProdCmd)
}
