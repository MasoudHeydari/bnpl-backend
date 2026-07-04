package command

import (
	"github.com/parstis/bnpl-backend/cmd/bnpl-dbadmin/bootstrap"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{Use: "ownership", Short: "Explicit managed ownership upgrade (drain writers first)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { return c.Help() }}
	cmd.AddCommand(&cobra.Command{Use: "plan", Short: "Read-only ownership preflight", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { return bootstrap.Ownership(c.OutOrStdout(), false) }})
	cmd.AddCommand(&cobra.Command{Use: "apply", Short: "Provision roles and transfer only bnpl managed ownership", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { return bootstrap.Ownership(c.OutOrStdout(), true) }})
	dbCmd.AddCommand(cmd)
}
