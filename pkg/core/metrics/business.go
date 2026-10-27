package metrics

import (
	"context"
	"time"
)

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

	// ShortLinkCreated records a persisted short link by creation mode
	// (generated or alias).
	ShortLinkCreated(ctx context.Context, mode string)

	// ShortLinkResolved records a short-link resolution outcome
	// (ok, unknown, expired, revoked, invalid, error).
	ShortLinkResolved(ctx context.Context, outcome string)

	// ShortLinkCodeCollision records a unique-code collision during creation.
	ShortLinkCodeCollision(ctx context.Context)

	// ShortLinkResolveLatency records short-link resolution latency.
	ShortLinkResolveLatency(ctx context.Context, duration time.Duration)

	// RepaymentReminderJobCompleted records one reminder dispatch job execution
	// outcome (success or failure).
	RepaymentReminderJobCompleted(ctx context.Context, outcome string)

	// RepaymentReminderJobDuration records reminder dispatch job duration.
	RepaymentReminderJobDuration(ctx context.Context, duration time.Duration)

	// RepaymentReminderCandidatesObserved records candidate installment count.
	RepaymentReminderCandidatesObserved(ctx context.Context, count int)

	// RepaymentReminderEligibleCustomersObserved records eligible customer count.
	RepaymentReminderEligibleCustomersObserved(ctx context.Context, count int)

	// RepaymentReminderCreated records a durably persisted reminder request by type.
	RepaymentReminderCreated(ctx context.Context, reminderType string)

	// RepaymentReminderSkipped records a skipped reminder by bounded reason.
	RepaymentReminderSkipped(ctx context.Context, reason string)
}
