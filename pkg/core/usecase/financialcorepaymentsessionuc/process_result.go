package financialcorepaymentsessionuc

// ProcessResult reports durable payment-session processing outcome at the use-case boundary.
type ProcessResult string

// Known durable payment-session processing outcomes.
const (
	ProcessResultSessionSubmitted        ProcessResult = "session_submitted"
	ProcessResultEventAlreadyProcessed   ProcessResult = "event_already_processed"
	ProcessResultPayloadConflictRecorded ProcessResult = "payload_conflict_recorded"
	ProcessResultPermanentlyRejected     ProcessResult = "permanently_rejected"
	ProcessResultSessionIdentityConflict ProcessResult = "session_identity_conflict"
)

// ProcessOutput reports durable payment-session processing.
type ProcessOutput struct {
	// Result reports the use-case outcome classification.
	Result ProcessResult
}
