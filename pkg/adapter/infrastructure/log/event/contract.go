package event

// Contract lifecycle checkpoints.
var (
	ContractPreparationStarted = define(
		"contract.preparation.started",
		"contract preparation started",
	)
	ContractPaymentPendingCreated = define(
		"contract.payment_pending.created",
		"payment_pending contract created",
	)
	ContractReplayed = define(
		"contract.replayed",
		"existing contract replayed",
	)
	ContractActivated = define(
		"contract.activated",
		"contract activated",
	)
	ContractCanceled = define(
		"contract.canceled",
		"contract canceled",
	)
	ContractExpired = define(
		"contract.expired",
		"contract expired",
	)
	ContractCompleted = define(
		"contract.completed",
		"contract completed",
	)
	ContractCancellationSagaStarted = define(
		"contract.cancellation.saga_started",
		"contract cancellation saga started",
	)
	ContractCancellationSagaCompleted = define(
		"contract.cancellation.saga_completed",
		"contract cancellation saga completed",
	)
	ContractCancellationSagaFailed = define(
		"contract.cancellation.saga_failed",
		"contract cancellation saga failed",
	)
	ContractPaymentPendingExpiryCompleted = define(
		"contract.payment_pending.expiry_completed",
		"payment_pending contract expiry sweep completed",
	)
)
