package command

import (
	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
	"github.com/spf13/cobra"
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
	return bootstrap.InitDev()
}

func init() {
	dbCmd.AddCommand(initDevCmd)
}
