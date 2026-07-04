package metrics

import "context"

// BusinessMetrics records bounded, low-cardinality BNPL business outcomes.
//
// Implementations must not use entity identifiers, idempotency keys, trace IDs,
// or other unbounded values as metric labels. Prefer enumerations such as
// outcome, status, channel, and from/to state.
//
// Callers must record observations only after durable success (committed
// transactions / applied inbox results). Idempotent replays must not increment
// counters again.
//
// BusinessMetrics is not a substitute for audit logging and is not a source of
// monetary truth. Financial Core remains the authority for payment state.
type BusinessMetrics interface {
	// CheckoutDecisionCreated records a newly persisted checkout decision.
	CheckoutDecisionCreated(ctx context.Context, outcome string)

	// ContractCreated records a newly persisted contract by initial status.
	ContractCreated(ctx context.Context, status string)

	// PaymentAttemptCompleted records a durably completed payment attempt.
	PaymentAttemptCompleted(ctx context.Context, result string, channel string)

	// InstallmentStateChanged records a committed installment lifecycle transition.
	InstallmentStateChanged(ctx context.Context, from string, to string)

	// WebhookDeliveryCompleted records the classified outcome of one webhook
	// delivery attempt (for example delivered, retryable_failure,
	// permanent_failure, destination_disabled).
	WebhookDeliveryCompleted(ctx context.Context, outcome string)
}
