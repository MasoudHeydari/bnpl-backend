package command

import "github.com/spf13/cobra"

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Run database administration commands",
	Long: `Run database administration commands for schema initialization and
migration workflows.

Use this command group to bootstrap a fresh database with init-dev or init-prod,
or to execute filesystem-backed SQL migrations with migrate.`,
}

func init() {
	rootCmd.AddCommand(dbCmd)
}
