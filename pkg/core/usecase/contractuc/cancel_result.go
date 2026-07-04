package contractuc

// CancelResult classifies contract cancellation start outcomes for HTTP mapping.
type CancelResult string

const (
	// CancelResultCompleted means the synchronous Financial Core cancellation
	// completed authoritatively and local state is now canceled.
	CancelResultCompleted CancelResult = "completed"

	// CancelResultStarted means active → canceling was persisted and the
	// cancellation Saga is running. Financial Core may still be retrying or the
	// local canceling → canceled finalizer may not have completed yet.
	CancelResultStarted CancelResult = "started"

	// CancelResultAlreadyCanceling means an exact idempotent replay while the
	// contract is already canceling.
	CancelResultAlreadyCanceling CancelResult = "already_canceling"

	// CancelResultAlreadyCanceled means an exact idempotent replay against a
	// contract that has already reached terminal canceled.
	CancelResultAlreadyCanceled CancelResult = "already_canceled"
)
