package paymentattemptexpirationuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records business flow checkpoints for local payment-attempt
// session-creation expiration. Implementations must not modify domain state or
// return errors that affect business execution.
//
// Per-attempt checkpoints are emitted only after the attempt's transaction
// committed. This use case never calls Financial Core, so no checkpoint here
// implies a payment outcome.
type Observer interface {
	// AttemptExpired records a committed local expiration of a payment attempt.
	AttemptExpired(ctx context.Context, paymentAttemptID model.PaymentAttemptID)

	// AttemptCanceled records a committed local cancellation of a payment
	// attempt whose session-creation retry budget was exhausted.
	AttemptCanceled(ctx context.Context, paymentAttemptID model.PaymentAttemptID)

	// ExpirationCycleCompleted records the end of one expiration cycle.
	ExpirationCycleCompleted(
		ctx context.Context,
		expiredCount int,
		failedCount int,
		duration time.Duration,
	)
}
