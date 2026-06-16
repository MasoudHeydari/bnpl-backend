package command

import (
	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply all configured SQL migration files",
	Long: `Apply all SQL migration files from the configured migration directory.

The command loads the application configuration, opens a normal-role PostgreSQL
connection pool, reads each migration file from disk, executes its SQL inside a
transaction, and registers the migration in the migrations table on success.`,
	RunE: migrate,
	Args: cobra.NoArgs,
}

func migrate(_ *cobra.Command, _ []string) error {
	return bootstrap.Migrate()
}

func init() {
	dbCmd.AddCommand(migrateCmd)
}
