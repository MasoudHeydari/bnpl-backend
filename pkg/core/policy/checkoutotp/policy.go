package checkoutotp

import (
	"fmt"
	"time"
)

// Policy exposes the security terms the checkout use case enforces.
type Policy interface {
	// Terms returns the immutable security terms.
	Terms() Terms
}

// Terms are the security limits of checkout phone OTP issuance and verification.
//
// Code format is a fixed core contract (six decimal digits via
// checkoutotp.GenerateSixDigitOTP / IsWellFormed), not a policy knob.
//
// Per-checkout terms bound one session. Cross-checkout terms bound abuse across
// many sessions that share a phone fingerprint or merchant: an attacker who can
// create sessions must still hit these distributed budgets. There is deliberately
// no IP- or device-based limit in this policy.
type Terms struct {
	// TTL is how long an issued code is accepted.
	TTL time.Duration

	// MaxVerificationAttempts is the wrong-code budget of one issued code.
	MaxVerificationAttempts int

	// SendCooldown is the minimum gap between two code deliveries on one checkout.
	SendCooldown time.Duration

	// MaxSendsPerCheckout is the total number of codes one checkout may issue.
	MaxSendsPerCheckout int

	// PhoneSendWindow is the fixed window used by the per-phone send budget.
	PhoneSendWindow time.Duration

	// MaxSendsPerPhone is how many new OTP issuances one phone fingerprint may
	// consume inside PhoneSendWindow across all checkouts.
	MaxSendsPerPhone int

	// MerchantSendWindow is the fixed window used by the per-merchant send budget.
	MerchantSendWindow time.Duration

	// MaxSendsPerMerchant is how many new OTP issuances one merchant may consume
	// inside MerchantSendWindow across all of that merchant's checkouts.
	//
	// V1 chooses a throughput ceiling that blocks runaway session spam without
	// capping ordinary partner traffic. Tune from production metrics later; keep
	// the number only here so use cases and repositories never hard-code it.
	MaxSendsPerMerchant int
}

// Validate reports whether every term is usable.
func (t Terms) Validate() error {
	const op = "checkoutotp.Terms.Validate"

	if t.TTL <= 0 {
		return fmt.Errorf("%s: ttl must be positive", op)
	}

	if t.MaxVerificationAttempts <= 0 {
		return fmt.Errorf("%s: max verification attempts must be positive", op)
	}

	if t.SendCooldown < 0 {
		return fmt.Errorf("%s: send cooldown must not be negative", op)
	}

	if t.MaxSendsPerCheckout <= 0 {
		return fmt.Errorf("%s: max sends per checkout must be positive", op)
	}

	if t.PhoneSendWindow <= 0 {
		return fmt.Errorf("%s: phone send window must be positive", op)
	}

	if t.MaxSendsPerPhone <= 0 {
		return fmt.Errorf("%s: max sends per phone must be positive", op)
	}

	if t.MerchantSendWindow <= 0 {
		return fmt.Errorf("%s: merchant send window must be positive", op)
	}

	if t.MaxSendsPerMerchant <= 0 {
		return fmt.Errorf("%s: max sends per merchant must be positive", op)
	}

	return nil
}

type policy struct {
	terms Terms
}

func (p policy) Terms() Terms {
	return p.terms
}

// V1 returns the first immutable OTP security policy.
//
// Per checkout: two-minute lifetime, five wrong-code attempts, a 60-second send
// cooldown, and at most three sends. Cross-checkout: five phone issuances per
// ten minutes, and a provisional 30,000 merchant issuances per ten minutes
// (runaway-abuse / SMS-cost ceiling — not normal throughput throttling; tune
// from production traffic and SMS metrics). Codes are always six decimal
// digits (core OTP contract). No IP or device limit.
func V1() Policy {
	return policy{
		terms: Terms{
			TTL:                     2 * time.Minute,
			MaxVerificationAttempts: 5,
			SendCooldown:            60 * time.Second,
			MaxSendsPerCheckout:     3,
			PhoneSendWindow:         10 * time.Minute,
			MaxSendsPerPhone:        5,
			MerchantSendWindow:      10 * time.Minute,
			// V1 safety ceiling only; replace with a metrics-backed value later.
			MaxSendsPerMerchant: 30_000,
		},
	}
}
