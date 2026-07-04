package model

import "time"

// MaxPaymentResultProcessingGracePeriod is the maximum operational processing grace
// period tolerated for delayed Financial Core payment-result delivery.
const MaxPaymentResultProcessingGracePeriod = 60 * time.Minute

// EffectivePaymentDeadline returns the earliest applicable payment deadline for a
// payment_pending contract from BNPL and Financial Core checkout session expiry.
func (c Contract) EffectivePaymentDeadline() time.Time {
	deadline := c.ExpiresAt.UTC()

	if c.FinancialPaymentExpiresAt != nil {
		financialDeadline := c.FinancialPaymentExpiresAt.UTC()
		if !financialDeadline.IsZero() && financialDeadline.Before(deadline) {
			deadline = financialDeadline
		}
	}

	return deadline
}

// ExpiryEligibleAt returns when a payment_pending contract becomes eligible for expiry
// after applying the operational payment-result processing grace period.
func (c Contract) ExpiryEligibleAt(processingGrace time.Duration) time.Time {
	return c.EffectivePaymentDeadline().Add(processingGrace)
}

// IsExpiryEligible reports whether now is at or after the contract expiry eligibility time.
func (c Contract) IsExpiryEligible(now time.Time, processingGrace time.Duration) bool {
	return !now.UTC().Before(c.ExpiryEligibleAt(processingGrace))
}

// PaymentOccurredOnTime reports whether paidAt occurred on or before the effective deadline.
func (c Contract) PaymentOccurredOnTime(paidAt time.Time) bool {
	if paidAt.IsZero() {
		return false
	}

	return !paidAt.UTC().After(c.EffectivePaymentDeadline())
}
