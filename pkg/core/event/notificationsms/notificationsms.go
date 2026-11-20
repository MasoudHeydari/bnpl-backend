// Package notificationsms defines BNPL outbound notification request envelopes
// published through the transactional outbox to Kafka.
//
// Repayment reminders and collections default notices publish Command envelopes
// to topic notification.commands.v1 with Kafka header
// x-notification-caller: bnpl-backend. Legacy Event / EventV2 payloads for
// notification.sms.requests are retained only for historical reference and
// must not be used for new reminder publishes.
//
// Notification Service resolves the authorized delivery destination from
// customer_id (CRS external id), renders SMS from template_code + variables,
// and deduplicates on idempotency_key. BNPL must not include phone numbers.
// Successful Kafka publication is not SMS delivery confirmation.
//
// Existing Outbox rows are never rewritten; the publisher republishes the
// durable payload bytes and headers unchanged.
package notificationsms
