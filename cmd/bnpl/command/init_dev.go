package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/config"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/usecase/migrationsuc"
)

var initDevCmd = &cobra.Command{
	Use:   "init-dev",
	Short: "Reset the database and load development bootstrap data",
	Long: `Reset the managed PostgreSQL schema and initialize it for development
use.

The command loads the application configuration, drops and recreates the managed
schema, ensures the application role and privileges exist, and then applies the
embedded schema and development seed SQL inside a transaction.`,
	RunE: initDev,
	Args: cobra.NoArgs,
}

func initDev(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}
	fmt.Printf("configs: %v\n", c)
	fmt.Println("Dev DB initializing...")

	muc := migrationsuc.NewInitDB(c)
	err = muc.InitDev(ctx)
	if err != nil {
		return fmt.Errorf("initializing DB with dev data: %w", err)
	}

	fmt.Println("Dev DB initialized.")
	return nil
}

func init() {
	dbCmd.AddCommand(initDevCmd)
}
