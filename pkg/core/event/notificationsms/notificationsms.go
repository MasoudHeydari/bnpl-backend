// Package notificationsms defines the BNPL outbound SMS notification request
// envelope published through the transactional outbox to Kafka.
//
// Notification Service is expected to:
//   - resolve the recipient phone number from ExternalCustomerID
//   - render the SMS using TemplateID and Variables
//   - process IdempotencyKey idempotently
//
// BNPL must not include phone numbers or other unnecessary PII in this event.
// Successful Kafka publication is not SMS delivery confirmation.
package notificationsms
