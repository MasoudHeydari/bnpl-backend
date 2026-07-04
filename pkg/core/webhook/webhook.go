// Package webhook defines the application-facing contracts for signed outbound
// merchant webhook delivery.
//
// It owns the logical outbox destination scheme, signature header names, and the
// delivery port. Destination-security rules live under pkg/core/policy/webhook.
// Concrete HTTP transport, dial-time SSRF enforcement, HMAC signing, and outbox
// publishing adapters live under pkg/adapter.
package webhook
