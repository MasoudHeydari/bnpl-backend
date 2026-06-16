package command

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	"github.com/spf13/cobra"
)

var initProdCmd = &cobra.Command{
	Use:   "init-prod",
	Short: "Reset the database and load production bootstrap data",
	Long: `Reset the managed PostgreSQL schema and initialize it for production
bootstrap.

The command loads the application configuration, drops and recreates the managed
schema, ensures the application role and privileges exist, and then applies the
embedded schema and production seed SQL inside a transaction.`,
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
	fmt.Println("Prod DB initializing...")

	muc := migrationsuc.NewInitDB(c)
	err = muc.InitProd(ctx)
	if err != nil {
		return fmt.Errorf("initializing DB with prod bootstrap data: %w", err)
	}

	fmt.Println("Prod DB initialized.")
	return nil
}

func init() {
	dbCmd.AddCommand(initProdCmd)
}
