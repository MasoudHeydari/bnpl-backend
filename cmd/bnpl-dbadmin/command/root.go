package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/parstis/bnpl-backend/cmd/bnpl-dbadmin/bootstrap"
)

var (
	cfgPath string
	rootCmd = &cobra.Command{
		Use:   "bnpl-dbadmin",
		Short: "Manage BNPL database schema, roles and integrity",
		// SilenceUsage/SilenceErrors keep operational failures to a single
		// stderr line from Execute; usage belongs to --help, not error paths.
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
)

// Execute runs the root command tree.
// It writes any terminal-facing error to stderr and exits the process with
// status code 1 when command execution fails.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(func() {
		bootstrap.FixConfigPath(cfgPath)
	})
	// Apply silence flags to the leaf command that actually runs; cobra does
	// not inherit SilenceUsage/SilenceErrors from the root command.
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	}
	rootCmd.PersistentFlags().StringVarP(
		&cfgPath,
		"config",
		"c",
		"",
		"config file path",
	)
}
