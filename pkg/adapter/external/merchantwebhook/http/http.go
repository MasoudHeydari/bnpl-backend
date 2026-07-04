// Package http implements signed outbound merchant webhook delivery over HTTP.
//
// It owns request construction, SSRF-safe dialing, HMAC signing, response
// classification, and the messaging.Publisher the outbox routes webhook://
// destinations through when external.webhook_dispatcher.provider is "http".
// Pure destination-security rules live in pkg/core/policy/webhook.
package http
