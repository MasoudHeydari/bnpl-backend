package event

// Messaging, outbox, inbox, and consumer checkpoints.
var (
	MessagingEventReceived = define(
		"messaging.event.received",
		"messaging event received",
	)
	MessagingEventAccepted = define(
		"messaging.event.accepted",
		"messaging event accepted",
	)
	MessagingEventSkipped = define(
		"messaging.event.skipped",
		"messaging event skipped",
	)
	MessagingDuplicateDetected = define(
		"messaging.event.duplicate_detected",
		"messaging duplicate detected",
	)
	MessagingProcessingStarted = define(
		"messaging.processing.started",
		"messaging processing started",
	)
	MessagingProcessingSucceeded = define(
		"messaging.processing.succeeded",
		"messaging processing succeeded",
	)
	MessagingProcessingFailed = define(
		"messaging.processing.failed",
		"messaging processing failed",
	)
	MessagingRetryScheduled = define(
		"messaging.retry.scheduled",
		"messaging retry scheduled",
	)
	MessagingRetryExhausted = define(
		"messaging.retry.exhausted",
		"messaging retry exhausted",
	)
	OutboxMessagePublished = define(
		"outbox.message.published",
		"outbox message published",
	)
	OutboxMessageSkipped = define(
		"outbox.message.skipped",
		"outbox message skipped",
	)
	OutboxMessageFailed = define(
		"outbox.message.failed",
		"outbox message permanently failed",
	)
	OutboxMessageRetryScheduled = define(
		"outbox.message.retry_scheduled",
		"outbox message retry scheduled",
	)
	OutboxLeaseLost = define(
		"outbox.lease.lost",
		"outbox lease lost",
	)
)
