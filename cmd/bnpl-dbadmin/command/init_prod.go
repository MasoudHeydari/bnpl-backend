//go:build production

package command

import (
	"github.com/spf13/cobra"

	"github.com/parstis/bnpl-backend/cmd/bnpl-dbadmin/bootstrap"
)

var initProdCmd = &cobra.Command{
	Use:   "init-prod",
	Short: "Create the production schema once when it does not already exist",
	Long: `Create managed schema bnpl for production bootstrap when it is missing.

Requires runtime.environment=production (APP_ENVIRONMENT=production).
If schema bnpl does not exist, this command creates it, ensures the application
role and privileges exist, and applies embedded schema DDL plus production seed
SQL inside a transaction.

If schema bnpl already exists, the command fails with a conflict error and does
not drop or mutate existing objects. Use db migrate (make db-migrate-production) for schema
evolution on an existing production database.`,
	RunE: initProd,
	Args: cobra.NoArgs,
}

func initProd(_ *cobra.Command, _ []string) error {
	return bootstrap.InitProd()
}

func init() {
	dbCmd.AddCommand(initProdCmd)
}
