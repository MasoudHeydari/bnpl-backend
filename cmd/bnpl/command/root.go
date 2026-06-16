// Package command implements the BNPL CLI adapter.
// It belongs to the adapter layer and translates command-line execution
// into configuration loading, HTTP server startup, and database operations.
// The package depends on application use cases and infrastructure adapters,
// but it does not define domain rules itself.
package command

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/routes"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
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
	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}
	fmt.Printf("configs: %v\n", c)
	p, err := c.ConnectionPool(ctx, repo.NormalRole)
	if err != nil {
		return fmt.Errorf("creating DB pool: %w", err)
	}
	defer func() {
		if err = p.Close(); err != nil {
			log.Printf("failed to close pool connection: %v", err)
		}
	}()

	var e = c.Gin.NewEngine()
	if err = routes.Register(ctx, e, p, c); err != nil {
		return fmt.Errorf("registering routes: %w", err)
	}
	if err = e.Run(c.Gin.GetListeningAddress()); err != nil {
		return fmt.Errorf("running Gin engine: %w", err)
	}
	return nil
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
	cobra.OnInitialize(fixConfigPath)
	rootCmd.PersistentFlags().StringVarP(
		&cfgPath, "config", "c", "", "config file path",
	)
}

// fixConfigPath resolves the configuration path once during BNPL startup.
// The function prefers an explicit CLI flag, then the CONFIG_FILE environment
// variable, and finally the repository-local default path.
func fixConfigPath() {
	if cfgPath != "" {
		return
	}
	var found bool
	if cfgPath, found = os.LookupEnv("CONFIG_FILE"); !found {
		cfgPath = "configs/config.yaml"
	}
}
