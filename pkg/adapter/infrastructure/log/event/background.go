package event

// Background jobs, sagas, and operational runtime checkpoints.
var (
	SagaStepStarted = define(
		"saga.step.started",
		"saga step started",
	)
	SagaStepCompleted = define(
		"saga.step.completed",
		"saga step completed",
	)
	SagaCompensationStarted = define(
		"saga.compensation.started",
		"saga compensation started",
	)
	SagaCompensationCompleted = define(
		"saga.compensation.completed",
		"saga compensation completed",
	)
	SagaProcessingCompleted = define(
		"saga.processing.completed",
		"due saga processing completed",
	)
	JobStarted = define(
		"job.started",
		"background job started",
	)
	JobFinished = define(
		"job.finished",
		"background job finished",
	)
	JobFailed = define(
		"job.failed",
		"background job failed",
	)
	TaskExecutionStarted = define(
		"task.execution.started",
		"task execution started",
	)
	TaskExecutionCompleted = define(
		"task.execution.completed",
		"task execution completed",
	)
	TaskExecutionFailed = define(
		"task.execution.failed",
		"task execution failed",
	)
	InstallmentLifecycleSweepCompleted = define(
		"installment.lifecycle.sweep_completed",
		"installment lifecycle sweep completed",
	)
	RepaymentReminderDispatchCompleted = define(
		"repayment.reminder.dispatch_completed",
		"repayment reminder dispatch completed",
	)
	CollectionsAssessmentCompleted = define(
		"collections.assessment.completed",
		"collections assessment completed",
	)
	PolicyAssignmentImportRowProcessed = define(
		"policy.assignment.import_row_processed",
		"policy assignment import row processed",
	)
	PolicyAssignmentImportRowFailed = define(
		"policy.assignment.import_row_failed",
		"policy assignment import row failed",
	)
	MerchantTerminalIssuanceCleanupCompleted = define(
		"merchant.terminal.issuance_cleanup_completed",
		"merchant terminal issuance cleanup completed",
	)
	ShortLinkResolved = define(
		"shortlink.resolved",
		"short link resolved",
	)
	CustomerAccountRetrieved = define(
		"customer.account.retrieved",
		"customer account retrieved",
	)
	MerchantProfileMutated = define(
		"merchant.profile.mutated",
		"merchant profile mutated",
	)
	MerchantBranchMutated = define(
		"merchant.branch.mutated",
		"merchant branch mutated",
	)
	MerchantTerminalMutated = define(
		"merchant.terminal.mutated",
		"merchant terminal mutated",
	)
	HTTPRequestCompleted = define(
		"http.request.completed",
		"http request completed",
	)
	ShutdownStarted = define(
		"ops.shutdown.started",
		"shutdown started",
	)
	ShutdownCompleted = define(
		"ops.shutdown.completed",
		"shutdown completed",
	)
	ShutdownTimedOut = define(
		"ops.shutdown.timed_out",
		"shutdown timed out",
	)
	MigrationApplied = define(
		"ops.migration.applied",
		"migration applied",
	)
	MigrationFailed = define(
		"ops.migration.failed",
		"migration failed",
	)
)
