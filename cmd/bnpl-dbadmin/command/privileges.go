package command

import (
	"fmt"

	"github.com/parstis/bnpl-backend/cmd/bnpl-dbadmin/bootstrap"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/invariants"
	"github.com/spf13/cobra"
)

func listPrivileges(cmd *cobra.Command) {
	for _, p := range invariants.PrivilegeRegistry() {
		fmt.Fprintf(cmd.OutOrStdout(), "%s.%s operations=%v update_columns=%v migration=%s\n  %s\n", p.Schema, p.Table, p.Operations, p.UpdateColumns, p.Migration, p.Description)
	}
}
func init() {
	cmd := &cobra.Command{Use: "privileges", Short: "Inspect reviewed effective Runtime privileges", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { return c.Help() }}
	cmd.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { listPrivileges(c); return nil }})
	cmd.AddCommand(&cobra.Command{Use: "verify", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { return bootstrap.VerifyDatabaseInvariants() }})
	dbCmd.AddCommand(cmd)
}
