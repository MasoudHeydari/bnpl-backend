// Package command implements the BNPL CLI adapter.
// It belongs to the adapter layer and translates command-line execution
// into configuration loading, HTTP server startup, and database operations.
// The package depends on application use cases and infrastructure adapters,
// but it does not define domain rules itself.
package command

import (
	"fmt"
	"os"

	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
	"github.com/spf13/cobra"
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
		RunE: startWebServer,
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
	cobra.OnInitialize(bootstrap.FixConfigPath)
	rootCmd.PersistentFlags().StringVarP(
		&cfgPath, "config", "c", "", "config file path",
	)
}
