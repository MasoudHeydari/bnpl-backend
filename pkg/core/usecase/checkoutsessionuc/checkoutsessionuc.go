// Package checkoutsessionuc implements the merchant checkout handoff boundary.
//
// A checkout session is the only way a merchant integration starts a BNPL
// checkout. External partner checkouts carry an encrypted customer phone and an
// opaque public token; the unauthenticated hosted page resolves Identity from
// that phone at START, evaluates financing, and authorizes Confirm with an SMS
// OTP bound to the offer's evaluation generation. Internal gateway checkouts
// are created already bound to a trusted customer. Merchant-facing responses
// never contain customer credit facts or risk reason codes.
//
// Trusted first-party callers may create a session and receive the same offer
// economics immediately, but only behind a stronger on-behalf capability.
package checkoutsessionuc
