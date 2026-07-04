//go:build development

package command

import (
	"github.com/spf13/cobra"

	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
)

var initDevCmd = &cobra.Command{
	Use:   "init-dev",
	Short: "Recreate the development schema and load development bootstrap data",
	Long: `Destructively reset managed schema bnpl and initialize it for development.

Requires runtime.environment=development (APP_ENVIRONMENT=development).
Whether the schema is empty or already populated, this command drops schema bnpl
(CASCADE), recreates it, ensures the application role and privileges exist, and
applies embedded schema DDL plus development seed SQL inside a transaction.

Do not run against production databases.`,
	RunE: initDev,
	Args: cobra.NoArgs,
}

func initDev(_ *cobra.Command, _ []string) error {
	return bootstrap.InitDev()
}

func init() {
	dbCmd.AddCommand(initDevCmd)
}
