package installmentrepaymentuc

// CreatePaymentAttemptResult identifies how payment attempt creation completed.
type CreatePaymentAttemptResult string

const (
	// CreatePaymentAttemptResultQueued indicates a new payment attempt was queued for Financial Core.
	CreatePaymentAttemptResultQueued CreatePaymentAttemptResult = "queued"

	// CreatePaymentAttemptResultAlreadyExists indicates an idempotent replay returned an existing attempt.
	CreatePaymentAttemptResultAlreadyExists CreatePaymentAttemptResult = "already_exists"
)
