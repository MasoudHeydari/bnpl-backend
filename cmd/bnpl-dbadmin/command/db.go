package command

import "github.com/spf13/cobra"

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Run database administration commands",
	Long:  dbCmdLong,
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

func init() {
	rootCmd.AddCommand(dbCmd)
}
