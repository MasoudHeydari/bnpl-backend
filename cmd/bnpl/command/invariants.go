package command

import (
	"fmt"

	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/invariants"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{Use: "invariants", Short: "Inspect managed database integrity rules", Args: cobra.NoArgs}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List invariant metadata without a database", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		for _, v := range invariants.Registry() {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n  %s.%s / %s -> %s.%s\n  migration=%s code=%s %s enabled=%s\n", v.Name, v.Description, v.Schema, v.Table, v.Trigger, v.Schema, v.Function, v.Migration, v.Code, v.EventTiming(), v.EnabledState)
		}
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "verify", Short: "Read-only verification of installed invariants", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := bootstrap.VerifyDatabaseInvariants(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "All managed database invariants verified.")
		return nil
	}})
	dbCmd.AddCommand(cmd)
}
