package model

// CheckoutOTPRateLimitScope identifies which cross-checkout OTP budget a
// durable counter row belongs to.
//
// phone keys are keyed HMAC fingerprints of a canonical E.164 phone.
// merchant keys are merchant profile UUID text. There is deliberately no IP
// or device scope.
type CheckoutOTPRateLimitScope string

const (
	// CheckoutOTPRateLimitScopePhone is the per-phone issuance budget.
	CheckoutOTPRateLimitScopePhone CheckoutOTPRateLimitScope = "phone"

	// CheckoutOTPRateLimitScopeMerchant is the per-merchant issuance budget.
	CheckoutOTPRateLimitScopeMerchant CheckoutOTPRateLimitScope = "merchant"
)

// IsValid reports whether scope is a known rate-limit scope.
func (s CheckoutOTPRateLimitScope) IsValid() bool {
	switch s {
	case CheckoutOTPRateLimitScopePhone, CheckoutOTPRateLimitScopeMerchant:
		return true

	default:
		return false
	}
}
