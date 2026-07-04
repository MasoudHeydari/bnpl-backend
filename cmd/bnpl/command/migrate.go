package command

import (
	"github.com/spf13/cobra"

	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply all configured SQL migration files",
	Long: `Apply all SQL migration files from the configured migration directory.

The command loads the application configuration, opens a normal-role PostgreSQL
connection pool, reads each migration file from disk, and applies it in lexical
order. Migrations run transactionally by default; explicitly non-transactional
migrations are tracked with a durable dirty marker until they finish successfully.`,
	RunE: migrate,
	Args: cobra.NoArgs,
}

func migrate(_ *cobra.Command, _ []string) error {
	return bootstrap.Migrate()
}

func init() {
	dbCmd.AddCommand(migrateCmd)
}
