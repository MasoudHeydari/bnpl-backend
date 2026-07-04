// Package checkoutorigin defines pure validation for the BNPL hosted-checkout
// public origin.
//
// The public origin is trusted deployment configuration — never derived from
// Host / X-Forwarded-* / request URL — because generated checkout URLs are
// returned to merchants and browsers. Production host pinning prevents a
// syntactically valid but incorrect environment value from redirecting
// customers to an unintended origin.
//
// Selecting Strict vs Development from runtime environment is Config/Bootstrap
// composition, not this package. Hosted checkout URL construction lives in
// pkg/core/checkouturl.
package checkoutorigin
