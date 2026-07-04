package model

// OTPRateLimitScope identifies which cross-subject OTP budget a durable
// counter row belongs to.
//
// phone keys are keyed HMAC fingerprints of a canonical E.164 phone.
// merchant keys are merchant profile UUID text. There is deliberately no IP
// or device scope.
type OTPRateLimitScope string

const (
	// OTPRateLimitScopePhone is the per-phone issuance budget.
	OTPRateLimitScopePhone OTPRateLimitScope = "phone"

	// OTPRateLimitScopeMerchant is the per-merchant issuance budget.
	OTPRateLimitScopeMerchant OTPRateLimitScope = "merchant"
)

// IsValid reports whether scope is a known rate-limit scope.
func (s OTPRateLimitScope) IsValid() bool {
	switch s {
	case OTPRateLimitScopePhone, OTPRateLimitScopeMerchant:
		return true

	default:
		return false
	}
}
