package paymentapplication

// ApplicationResult identifies how BNPL applied a Financial Core payment result.
type ApplicationResult string

// Known payment application results.
const (
	AppliedApplicationResult                ApplicationResult = "applied"
	AlreadyAppliedApplicationResult         ApplicationResult = "already_applied"
	ReconciliationRequiredApplicationResult ApplicationResult = "reconciliation_required"
)

// IsValid reports whether the application result is known by the domain.
func (r ApplicationResult) IsValid() bool {
	switch r {
	case AppliedApplicationResult,
		AlreadyAppliedApplicationResult,
		ReconciliationRequiredApplicationResult:
		return true

	default:
		return false
	}
}
