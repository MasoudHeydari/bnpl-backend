package notification

import (
	"context"
	"time"
)

// Submitter durably submits notifications through the Notification Service.
//
// Implementations perform network I/O and must never be called inside a
// database transaction. A successful Submit means Notification accepted and
// stored the request, not that a provider delivered the message.
type Submitter interface {
	Submit(ctx context.Context, input SubmitInput) (SubmitOutput, error)
}

// SubmitInput carries one notification submission.
type SubmitInput struct {
	// TemplateID identifies the logical Notification template.
	TemplateID string

	// Recipient is the canonical destination address (E.164 for SMS OTP).
	Recipient string

	// Variables are template placeholders by logical name. Values may carry
	// secrets (such as an OTP) and must never be logged.
	Variables map[string]string

	// IdempotencyKey is the caller-owned replay identity for this submission.
	IdempotencyKey string

	// CorrelationID is tracing-only and is not part of idempotency.
	CorrelationID string

	// ExpiresAt is the authoritative caller-owned expiry. For checkout OTP it
	// must equal BNPL's OTPExpiresAt for the issued challenge.
	ExpiresAt time.Time
}

// SubmitOutput carries a durable Notification acceptance result.
type SubmitOutput struct {
	// RequestID is the Notification Service request identifier.
	RequestID string

	// Duplicate is true when Notification replayed an already-accepted request
	// for the same idempotency key and payload.
	Duplicate bool
}
