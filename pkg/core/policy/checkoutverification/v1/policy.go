package v1

import (
	"fmt"
	"time"
)

const (
	// PolicyID is the stable merchant verification policy family identifier.
	PolicyID = "checkout_merchant_verification"

	// PolicyVersion is this compile-time revision label.
	PolicyVersion = "v1"

	// VerifyTimeout is how long a merchant may call Verify after BNPL records
	// durable Financial Core success and enters verify_pending.
	VerifyTimeout = 30 * time.Minute
)

// Policy is the v1 merchant verification policy.
type Policy struct{}

// New returns the v1 merchant verification policy.
func New() Policy {
	return Policy{}
}

// ID returns the stable policy family identifier.
func (Policy) ID() string { return PolicyID }

// Version returns this policy revision.
func (Policy) Version() string { return PolicyVersion }

// DeadlineAt returns the exclusive verify deadline from the instant BNPL
// recorded financial success as verify_pending.
func (Policy) DeadlineAt(verificationPendingAt time.Time) time.Time {
	return verificationPendingAt.UTC().Add(VerifyTimeout)
}

// AllowsVerifyAt reports whether now is strictly before deadline.
// At the exact deadline instant, verify is rejected.
func (Policy) AllowsVerifyAt(now, deadline time.Time) bool {
	return now.UTC().Before(deadline.UTC())
}

// Validate reports whether the fixed terms are usable.
func (Policy) Validate() error {
	const op = "checkoutverification.v1.Policy.Validate"

	if VerifyTimeout <= 0 {
		return fmt.Errorf("%s: verify timeout must be positive", op)
	}

	return nil
}
