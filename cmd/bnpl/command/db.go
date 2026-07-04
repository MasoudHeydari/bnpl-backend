package command

import "github.com/spf13/cobra"

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Run database administration commands",
	Long:  dbCmdLong,
}

func init() {
	rootCmd.AddCommand(dbCmd)
}
