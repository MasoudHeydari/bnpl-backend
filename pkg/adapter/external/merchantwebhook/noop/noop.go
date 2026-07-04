// Package noop provides a messaging.Publisher that acknowledges webhook outbox
// destinations without performing HTTP delivery.
//
// It is selected explicitly via configuration and must not be enabled in
// production. Kafka and other non-webhook destinations are unaffected because
// the messaging router only forwards webhook:// prefixes here.
package noop
