package event

// Payment attempt, Financial Core, and reconciliation checkpoints.
var (
	PaymentAttemptCreated = define(
		"payment.attempt.created",
		"payment attempt created",
	)
	PaymentAttemptReplayed = define(
		"payment.attempt.replayed",
		"payment attempt idempotent replay",
	)
	PaymentFinancialCoreRequestStarted = define(
		"payment.financial_core.request_started",
		"financial core payment session request started",
	)
	PaymentFinancialCoreResponseReceived = define(
		"payment.financial_core.response_received",
		"financial core payment session response received",
	)
	PaymentURLObtained = define(
		"payment.url.obtained",
		"payment url obtained",
	)
	PaymentFinancialCoreTimedOut = define(
		"payment.financial_core.timed_out",
		"financial core payment session timed out",
	)
	PaymentFinancialCoreFailed = define(
		"payment.financial_core.failed",
		"financial core payment session failed",
	)
	PaymentResultReceived = define(
		"payment.result.received",
		"payment result event received",
	)
	PaymentResultValidated = define(
		"payment.result.validated",
		"payment result validated",
	)
	PaymentResultValidationFailed = define(
		"payment.result.validation_failed",
		"payment result validation failed",
	)
	PaymentInboxDuplicateDetected = define(
		"payment.inbox.duplicate_detected",
		"payment inbox duplicate detected",
	)
	PaymentApplicationStarted = define(
		"payment.application.started",
		"payment application started",
	)
	PaymentApplicationSucceeded = define(
		"payment.application.succeeded",
		"payment application succeeded",
	)
	PaymentApplicationAlreadyApplied = define(
		"payment.application.already_applied",
		"payment result already applied",
	)
	PaymentApplicationConflict = define(
		"payment.application.conflict",
		"payment application conflict",
	)
	PaymentFirstInstallmentMarkedPaid = define(
		"payment.installment.first_marked_paid",
		"first installment marked paid",
	)
	PaymentTerminalFailure = define(
		"payment.attempt.terminal_failure",
		"payment attempt reached terminal failure",
	)
	PaymentReconciliationMismatch = define(
		"payment.reconciliation.mismatch",
		"payment reconciliation mismatch",
	)
	PaymentReconciliationRetryScheduled = define(
		"payment.reconciliation.retry_scheduled",
		"payment reconciliation retry scheduled",
	)
	PaymentReconciliationSucceeded = define(
		"payment.reconciliation.succeeded",
		"payment reconciliation succeeded",
	)
	PaymentReconciliationFailed = define(
		"payment.reconciliation.failed",
		"payment reconciliation failed",
	)
	PaymentAttemptExpirationCompleted = define(
		"payment.attempt.expiration_completed",
		"payment attempt expiration cycle completed",
	)
	InstallmentRepaymentRequested = define(
		"payment.repayment.requested",
		"installment repayment requested",
	)
	InstallmentRepaymentApplied = define(
		"payment.repayment.applied",
		"installment repayment applied",
	)
	InstallmentRepaymentFailed = define(
		"payment.repayment.failed",
		"installment repayment failed",
	)
)
