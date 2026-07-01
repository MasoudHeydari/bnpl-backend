package command

import (
	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
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
	return bootstrap.InitProd()
}

func init() {
	dbCmd.AddCommand(initProdCmd)
}
