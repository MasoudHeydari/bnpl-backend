package command

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/config"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/restful/gin/routes"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

var (
	cfgPath string
	rootCmd = &cobra.Command{
		Use:   "bnpl",
		Short: "",
		Long:  "",
		RunE:  startWebServer,
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
	defer p.Close()
	var e *gin.Engine = c.Gin.NewEngine()
	if err = routes.Register(ctx, e, p, c); err != nil {
		return fmt.Errorf("registering routes: %w", err)
	}
	if err = e.Run(c.Gin.GetListeningAddress()); err != nil {
		return fmt.Errorf("running Gin engine: %w", err)
	}
	return nil
}

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

func fixConfigPath() {
	if cfgPath != "" {
		return
	}
	var found bool
	if cfgPath, found = os.LookupEnv("CONFIG_FILE"); !found {
		// the default path should usually be in the /etc directory
		cfgPath = "configs/config.yaml"
	}
}
