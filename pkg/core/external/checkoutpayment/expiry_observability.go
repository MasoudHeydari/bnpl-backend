package checkoutpayment

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/log"
)

// LatePaymentSessionExpiryWarning is the structured warning message emitted when
// Financial Core returns a PaymentSession expiration later than BNPL requested.
const LatePaymentSessionExpiryWarning = "financial core returned payment session expiry later than requested"

// WarnIfExpiresAtLaterThanRequested logs a warning when FC's authoritative
// session expiry is later than the BNPL-requested deadline. This is a soft
// contract deviation: the response remains acceptable when hard session
// invariants already passed.
func WarnIfExpiresAtLaterThanRequested(
	logger log.Logger,
	requestedExpiresAt time.Time,
	fcExpiresAt time.Time,
	attrs ...any,
) {
	requested := requestedExpiresAt.UTC()
	actual := fcExpiresAt.UTC()
	if !actual.After(requested) {
		return
	}

	args := make([]any, 0, len(attrs)+6)
	args = append(args, attrs...)
	args = append(args,
		"requested_expires_at", requested.Format(time.RFC3339Nano),
		"fc_expires_at", actual.Format(time.RFC3339Nano),
		"expiry_deviation_seconds", actual.Sub(requested).Seconds(),
	)
	logger.Warn(LatePaymentSessionExpiryWarning, args...)
}
