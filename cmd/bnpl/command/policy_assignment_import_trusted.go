package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
)

var (
	trustedImportFile  string
	trustedImportActor string
)

var importTrustedPolicyAssignmentsCmd = &cobra.Command{
	Use:   "import-trusted",
	Short: "Bulk-assign the trusted customer policy bundle from authoritative subject IDs",
	Long: `Bulk-import trusted customer policy assignments from a CSV file.

The CSV must contain exactly one column named subject_id. Values are stable
Identity Service subject IDs, not national IDs. Example:

subject_id
550e8400-e29b-41d4-a716-446655440000
92d2c7b2-6f8b-4f25-8bb2-4deab49af151

The CSV is the operational source of truth for trusted membership. BNPL does
not call Identity Service during import. Every structurally valid subject
atomically receives the BNPL trusted policy bundle (checkout/v2 and
initial-credit-limit/v2). Persistence failures are recorded as durable failed
import items and do not roll back successful rows.`,
	Args: cobra.NoArgs,
	RunE: importTrustedPolicyAssignments,
}

func importTrustedPolicyAssignments(cmd *cobra.Command, _ []string) error {
	output, err := bootstrap.ImportTrustedPolicyAssignments(trustedImportFile, trustedImportActor)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(
		cmd.OutOrStdout(),
		"policy assignment import %s completed: success=%d failed=%d\n",
		output.Import.ID.String(),
		output.SuccessCount,
		output.FailureCount,
	)
	return err
}

func init() {
	importTrustedPolicyAssignmentsCmd.Flags().StringVar(
		&trustedImportFile, "file", "", "CSV file with exactly one column: subject_id",
	)
	importTrustedPolicyAssignmentsCmd.Flags().StringVar(
		&trustedImportActor, "actor", "", "operator identity recorded in assignment/import audit history",
	)
	_ = importTrustedPolicyAssignmentsCmd.MarkFlagRequired("file")
	_ = importTrustedPolicyAssignmentsCmd.MarkFlagRequired("actor")
	policyAssignmentCmd.AddCommand(importTrustedPolicyAssignmentsCmd)
}
