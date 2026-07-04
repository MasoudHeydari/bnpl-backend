// Package checkouturl builds BNPL hosted-checkout page URLs from a validated
// public origin.
//
// The path `/checkout/{checkout_session_id}` is owned by application code.
// Configuration supplies only the origin (see pkg/core/policy/checkoutorigin);
// callers must never concatenate raw environment strings.
package checkouturl
