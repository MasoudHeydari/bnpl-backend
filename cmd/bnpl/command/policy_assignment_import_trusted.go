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
	Short: "Bulk-assign the trusted customer policy bundle from Customer Reference mobiles",
	Long: `Bulk-import trusted customer policy assignments from a CSV file.

The CSV must contain exactly one column named mobile_e164. Values are Iranian
mobile numbers in E.164 (+989…) or local (09…) form. Example:

mobile_e164
+989121234567
+989351234567
09191234567

Each row is validated, resolved through Customer Reference Service
(ResolveCustomerByMobile), and — only when status is ACTIVE — atomically
receives the BNPL trusted policy bundle (checkout/v2 and
initial-credit-limit/v2). Importing does not grant credit or create contracts;
existing credit/checkout flows apply the trusted policies later.

The import is additive and idempotent: customers omitted from the CSV stay
trusted, and already-trusted customers are reported as already_exists.
Row-level failures (invalid mobile, not found, non-ACTIVE status, Customer
Service errors, persistence errors) are logged and recorded without stopping
the rest of the file.`,
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
		&trustedImportFile, "file", "", "CSV file with exactly one column: mobile_e164",
	)
	importTrustedPolicyAssignmentsCmd.Flags().StringVar(
		&trustedImportActor, "actor", "", "operator identity recorded in assignment/import audit history",
	)
	_ = importTrustedPolicyAssignmentsCmd.MarkFlagRequired("file")
	_ = importTrustedPolicyAssignmentsCmd.MarkFlagRequired("actor")
	policyAssignmentCmd.AddCommand(importTrustedPolicyAssignmentsCmd)
}
