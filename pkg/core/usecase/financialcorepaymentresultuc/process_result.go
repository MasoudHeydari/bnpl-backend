package financialcorepaymentresultuc

// ProcessResult reports durable payment-result processing outcome at the use-case boundary.
type ProcessResult string

// Known durable payment-result processing outcomes.
const (
	ProcessResultPaymentApplied          ProcessResult = "payment_applied"
	ProcessResultAlreadyApplied          ProcessResult = "already_applied"
	ProcessResultReconciliationRequired  ProcessResult = "reconciliation_required"
	ProcessResultFailureRecorded         ProcessResult = "failure_recorded"
	ProcessResultCancellationRecorded    ProcessResult = "cancellation_recorded"
	ProcessResultEventAlreadyProcessed   ProcessResult = "event_already_processed"
	ProcessResultPayloadConflictRecorded ProcessResult = "payload_conflict_recorded"
	ProcessResultTerminalResultUnmatched ProcessResult = "terminal_result_unmatched"
	ProcessResultPermanentlyRejected     ProcessResult = "permanently_rejected"
)

// ProcessOutput reports durable payment result processing.
type ProcessOutput struct {
	// Result reports the use-case outcome classification.
	Result ProcessResult
}
