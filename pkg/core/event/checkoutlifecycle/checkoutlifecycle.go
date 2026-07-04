// Package checkoutlifecycle defines merchant-facing checkout session lifecycle
// events published through the transactional outbox.
//
// These events are the authoritative merchant notification channel for checkout
// outcomes. Browser return query parameters are UX only and must never be
// treated as payment or checkout truth. Payment attempt internals are never
// exposed on this contract.
package checkoutlifecycle
