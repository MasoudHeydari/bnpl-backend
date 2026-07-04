package command

import "github.com/spf13/cobra"

var policyAssignmentCmd = &cobra.Command{
	Use:   "policy-assignment",
	Short: "Manage runtime policy assignments",
	Long: `Run operational commands for runtime policy assignment workflows.

Use import-trusted to bulk-assign the trusted customer policy bundle from an
authoritative CSV of stable subject IDs.`,
}

func init() {
	rootCmd.AddCommand(policyAssignmentCmd)
}
