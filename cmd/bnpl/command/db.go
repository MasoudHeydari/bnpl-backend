package command

import "github.com/spf13/cobra"

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management actions",
	Long: `Database management actions can be chosen by sub-commands.
For fresh installation in a development or production environment,
the 'init-dev' or 'init-prod' may be used and for migration operations,
the 'migrate' may be used.`,
}

func init() {
	rootCmd.AddCommand(dbCmd)
}
