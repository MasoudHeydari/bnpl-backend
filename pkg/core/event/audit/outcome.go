package audit

// Outcome identifies the result of an audited action.
type Outcome string

// Known outcomes for audit events.
const (
	SucceededOutcome Outcome = "succeeded"
	FailedOutcome    Outcome = "failed"
	DeniedOutcome    Outcome = "denied"
)

// IsValid reports whether the outcome is known by the audit contract.
func (o Outcome) IsValid() bool {
	switch o {
	case SucceededOutcome, FailedOutcome, DeniedOutcome:
		return true

	default:
		return false
	}
}
