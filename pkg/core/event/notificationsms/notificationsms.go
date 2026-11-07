// Package notificationsms defines BNPL outbound notification request envelopes
// published through the transactional outbox to Kafka.
//
// Two payload schemas are supported:
//   - Schema 1 (legacy): Event with TemplateID / Recipient.CustomerID
//   - Schema 2: EventV2 Notification Service envelope (targets + deliveries)
//
// Notification Service is expected to:
//   - resolve the authorized delivery destination from ExternalCustomerID
//   - render the SMS using the selected template and Variables
//   - process IdempotencyKey idempotently
//
// BNPL must not include phone numbers or other unnecessary PII in these events.
// Successful Kafka publication is not SMS delivery confirmation.
//
// Schema 2 is opt-in via repayment reminder configuration. Default remains
// schema 1 until Notification Service confirms v2 consumer support. Existing
// Outbox rows are never rewritten; the publisher republishes the durable
// payload bytes unchanged.
package notificationsms
