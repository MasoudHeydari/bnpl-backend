package checkouturl

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutorigin "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutorigin"
)

const checkoutPathPrefix = "/checkout/"

// Builder builds BNPL hosted-checkout URLs from a validated PublicOrigin.
type Builder struct {
	origin checkoutorigin.PublicOrigin
}

// New returns a builder bound to origin.
//
// origin must be non-zero (produced by checkoutorigin.Policy.Parse). Zero
// origins are rejected by callers that construct use cases.
func New(origin checkoutorigin.PublicOrigin) Builder {
	return Builder{origin: origin}
}

// Origin returns the bound public origin.
func (b Builder) Origin() checkoutorigin.PublicOrigin {
	return b.origin
}

// IsZero reports whether the builder has no validated origin.
func (b Builder) IsZero() bool {
	return b.origin.IsZero()
}

// Checkout returns the hosted checkout page URL for token.
//
// token is the typed public CheckoutSessionToken; the internal
// CheckoutSessionID never appears in a URL, and arbitrary path strings cannot
// be passed. The origin never contributes a path, so this never introduces a
// duplicate slash.
func (b Builder) Checkout(token model.CheckoutSessionToken) string {
	if b.origin.IsZero() || token.IsZero() {
		return ""
	}

	return b.origin.String() + checkoutPathPrefix + token.String()
}
