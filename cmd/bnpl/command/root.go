package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
)

var (
	cfgPath string
	rootCmd = &cobra.Command{
		Use:   "bnpl",
		Short: "Start the BNPL HTTP server",
		Long: `Start the BNPL application HTTP server using the configured
database and Gin settings.

When run without a subcommand, bnpl loads the configuration file, opens a
normal-role PostgreSQL connection pool, registers the HTTP routes, and starts
listening on the configured Gin address.`,
		// SilenceUsage/SilenceErrors keep operational failures to a single
		// stderr line from Execute; usage belongs to --help, not error paths.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          startWebServer,
	}
)

func startWebServer(_ *cobra.Command, _ []string) error {
	return bootstrap.Boot()
}

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
