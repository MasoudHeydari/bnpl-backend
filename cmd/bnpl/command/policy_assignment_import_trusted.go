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
	Short: "Import immutable trusted mobile/national-ID pairs without external calls",
	Long: `Import trusted phone/national-ID memberships from a two-column CSV.

The CSV header must be exactly: mobile_e164,national_id. Example:

mobile_e164,national_id
09121234567,0012345678
+989351234567,1234567891

Mobile numbers are normalized to Iranian E.164. The canonical raw mobile and
national ID are stored as an immutable pair. Import never calls Customer
Reference Service, assigns credit, or changes checkout policy. At first credit
assignment, the current mobile's national ID must be checked against CRS;
a mismatch must be rejected as a conflict.

The import is additive and idempotent: repeating an identical pair leaves it
unchanged, while importing an existing mobile with a different national ID is
reported as a conflict rather than silently replacing it.`,
	Args: cobra.NoArgs,
	RunE: importTrustedPolicyAssignments,
}

func importTrustedPolicyAssignments(cmd *cobra.Command, _ []string) error {
	output, err := bootstrap.ImportTrustedPolicyAssignments(trustedImportFile, trustedImportActor)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), output.FormatSummary())
	return err
}

func init() {
	importTrustedPolicyAssignmentsCmd.Flags().StringVar(
		&trustedImportFile, "file", "", "CSV file with exactly two columns: mobile_e164,national_id",
	)
	importTrustedPolicyAssignmentsCmd.Flags().StringVar(
		&trustedImportActor, "actor", "", "operator identity recorded in assignment/import audit history",
	)
	_ = importTrustedPolicyAssignmentsCmd.MarkFlagRequired("file")
	_ = importTrustedPolicyAssignmentsCmd.MarkFlagRequired("actor")
	policyAssignmentCmd.AddCommand(importTrustedPolicyAssignmentsCmd)
}
