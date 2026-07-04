package notificationsms

import (
	"fmt"
	"strings"
	"time"
)

const (
	// SchemaVersionV2 is the Notification Service JSON envelope version.
	SchemaVersionV2 = 2

	// EventTypeV2 is the Outbox / Kafka machine-readable event type for schema
	// v2 reminder notification requests. It is distinct from NotificationType
	// inside the payload.
	EventTypeV2 = "notification.requested.v2"

	// CategoryTransactional marks repayment and collections default notices as
	// transactional deliveries.
	CategoryTransactional = "transactional"

	// BusinessScopeBNPL identifies BNPL as the owning business scope.
	BusinessScopeBNPL = "bnpl"

	// BusinessReferenceTypeInstallment is the business reference kind for every
	// installment-scoped reminder event.
	BusinessReferenceTypeInstallment = "installment"

	// TargetKeyCustomer identifies the single customer target within an event.
	TargetKeyCustomer = "customer"

	// TargetTypeCustomer identifies a Customer Reference Service customer target.
	TargetTypeCustomer = "customer"

	// DeliveryMethodSMS is the delivery method for reminder SMS.
	DeliveryMethodSMS = "sms"

	// ChannelSMS is the channel for reminder SMS.
	ChannelSMS = "sms"
)

// EventV2 is the Notification Service schema v2 envelope for repayment reminder
// and collections default SMS requests.
//
// ExternalCustomerID on each target must be the CRS id (Customer Reference
// Service), never customer_profiles.id and never an Identity subject ID.
// Phone numbers and recipient hashes must not appear in this payload.
type EventV2 struct {
	// SchemaVersion is the Notification envelope version (always 2).
	SchemaVersion int `json:"schema_version"`

	// SourceService identifies BNPL as the producer.
	SourceService string `json:"source_service"`

	// SourceEventID is the durable BNPL event identity generated at persist time.
	SourceEventID string `json:"source_event_id"`

	// IdempotencyKey is the stable Notification deduplication key.
	IdempotencyKey string `json:"idempotency_key"`

	// NotificationType is the business notification type / template family.
	NotificationType string `json:"notification_type"`

	// Category classifies the notification (transactional for reminders).
	Category string `json:"category"`

	// CorrelationID is a UUID correlating this notification request.
	CorrelationID string `json:"correlation_id"`

	// ExpiresAt is the UTC delivery deadline. Attempts after this instant are
	// rejected by Notification Service; Kafka publication alone does not enforce it.
	ExpiresAt time.Time `json:"expires_at"`

	// Targets lists delivery targets (one customer target for reminder SMS).
	Targets []TargetV2 `json:"targets"`
}

// TargetV2 identifies one notification target and its deliveries.
type TargetV2 struct {
	// TargetKey identifies this target within the event.
	TargetKey string `json:"target_key"`

	// TargetType identifies the kind of target.
	TargetType string `json:"target_type"`

	// ExternalCustomerID is the Customer Reference Service external customer id.
	ExternalCustomerID string `json:"customer_id"`

	// BusinessScope is the owning business scope (bnpl).
	BusinessScope string `json:"business_scope"`

	// BusinessReferenceType is the referenced business entity kind.
	BusinessReferenceType string `json:"business_reference_type"`

	// BusinessReferenceID is the referenced business entity id (installment).
	BusinessReferenceID string `json:"business_reference_id"`

	// Deliveries lists channel deliveries for this target.
	Deliveries []DeliveryV2 `json:"deliveries"`
}

// DeliveryV2 describes one channel delivery attempt specification.
type DeliveryV2 struct {
	// DeliveryMethod is the delivery method (sms).
	DeliveryMethod string `json:"delivery_method"`

	// Channel is the delivery channel (sms).
	Channel string `json:"channel"`

	// TemplateCode selects the Notification template.
	TemplateCode string `json:"template_code"`

	// Variables are non-PII template substitutions (for example payment_url).
	Variables map[string]string `json:"variables"`
}

// EventV2Input carries the durable fields required to construct EventV2 once.
type EventV2Input struct {
	// SourceEventID is the durable BNPL event id.
	SourceEventID string

	// IdempotencyKey is the semantic Notification idempotency key.
	IdempotencyKey string

	// CorrelationID is a UUID correlation id.
	CorrelationID string

	// NotificationType is the business notification / template identifier.
	NotificationType string

	// ExternalCustomerID is the Customer Reference Service customer id.
	ExternalCustomerID string

	// InstallmentID is the installment business reference.
	InstallmentID string

	// PaymentURL is the short-link payment URL variable.
	PaymentURL string

	// ExpiresAt is the UTC delivery deadline.
	ExpiresAt time.Time
}

// NewEventV2 constructs a schema v2 notification request envelope.
func NewEventV2(input EventV2Input) EventV2 {
	vars := map[string]string{
		"payment_url": input.PaymentURL,
	}

	return EventV2{
		SchemaVersion:    SchemaVersionV2,
		SourceService:    ProducerName,
		SourceEventID:    input.SourceEventID,
		IdempotencyKey:   input.IdempotencyKey,
		NotificationType: input.NotificationType,
		Category:         CategoryTransactional,
		CorrelationID:    input.CorrelationID,
		ExpiresAt:        input.ExpiresAt.UTC(),
		Targets: []TargetV2{{
			TargetKey:             TargetKeyCustomer,
			TargetType:            TargetTypeCustomer,
			ExternalCustomerID:    input.ExternalCustomerID,
			BusinessScope:         BusinessScopeBNPL,
			BusinessReferenceType: BusinessReferenceTypeInstallment,
			BusinessReferenceID:   input.InstallmentID,
			Deliveries: []DeliveryV2{{
				DeliveryMethod: DeliveryMethodSMS,
				Channel:        ChannelSMS,
				TemplateCode:   input.NotificationType,
				Variables:      vars,
			}},
		}},
	}
}

// ValidateEventV2 validates a schema v2 notification request before outbox insertion.
func ValidateEventV2(event EventV2) error {
	const op = "notificationsms.ValidateEventV2"

	if event.SchemaVersion != SchemaVersionV2 {
		return fmt.Errorf("%s: schema_version must be %d", op, SchemaVersionV2)
	}

	if strings.TrimSpace(event.SourceService) != ProducerName {
		return fmt.Errorf("%s: source_service must be %q", op, ProducerName)
	}

	if strings.TrimSpace(event.SourceEventID) == "" {
		return fmt.Errorf("%s: source_event_id is required", op)
	}

	if strings.TrimSpace(event.IdempotencyKey) == "" {
		return fmt.Errorf("%s: idempotency_key is required", op)
	}

	if strings.TrimSpace(event.NotificationType) == "" {
		return fmt.Errorf("%s: notification_type is required", op)
	}

	if event.Category != CategoryTransactional {
		return fmt.Errorf("%s: category must be %q", op, CategoryTransactional)
	}

	if strings.TrimSpace(event.CorrelationID) == "" {
		return fmt.Errorf("%s: correlation_id is required", op)
	}

	if event.ExpiresAt.IsZero() {
		return fmt.Errorf("%s: expires_at is required", op)
	}

	if len(event.Targets) != 1 {
		return fmt.Errorf("%s: exactly one target is required", op)
	}

	target := event.Targets[0]
	if target.TargetKey != TargetKeyCustomer {
		return fmt.Errorf("%s: target_key must be %q", op, TargetKeyCustomer)
	}

	if target.TargetType != TargetTypeCustomer {
		return fmt.Errorf("%s: target_type must be %q", op, TargetTypeCustomer)
	}

	if strings.TrimSpace(target.ExternalCustomerID) == "" {
		return fmt.Errorf("%s: customer_id is required", op)
	}

	if target.BusinessScope != BusinessScopeBNPL {
		return fmt.Errorf("%s: business_scope must be %q", op, BusinessScopeBNPL)
	}

	if target.BusinessReferenceType != BusinessReferenceTypeInstallment {
		return fmt.Errorf("%s: business_reference_type must be %q", op, BusinessReferenceTypeInstallment)
	}

	if strings.TrimSpace(target.BusinessReferenceID) == "" {
		return fmt.Errorf("%s: business_reference_id is required", op)
	}

	if len(target.Deliveries) != 1 {
		return fmt.Errorf("%s: exactly one delivery is required", op)
	}

	delivery := target.Deliveries[0]
	if delivery.DeliveryMethod != DeliveryMethodSMS {
		return fmt.Errorf("%s: delivery_method must be %q", op, DeliveryMethodSMS)
	}

	if delivery.Channel != ChannelSMS {
		return fmt.Errorf("%s: channel must be %q", op, ChannelSMS)
	}

	if strings.TrimSpace(delivery.TemplateCode) == "" {
		return fmt.Errorf("%s: template_code is required", op)
	}

	if delivery.TemplateCode != event.NotificationType {
		return fmt.Errorf("%s: template_code must equal notification_type", op)
	}

	if delivery.Variables == nil {
		return fmt.Errorf("%s: variables must not be nil", op)
	}

	if strings.TrimSpace(delivery.Variables["payment_url"]) == "" {
		return fmt.Errorf("%s: payment_url variable is required", op)
	}

	if _, hasPhone := delivery.Variables["recipient_address"]; hasPhone {
		return fmt.Errorf("%s: recipient_address must not be present", op)
	}

	if _, hasHash := delivery.Variables["recipient_hash"]; hasHash {
		return fmt.Errorf("%s: recipient_hash must not be present", op)
	}

	return nil
}

// DeliveryDeadlineForBusinessDate returns the start of the next calendar day
// after businessDate in businessDate's location, expressed in UTC.
//
// businessDate must already be truncated to the start of a business day in the
// configured business timezone. Do not approximate this with a fixed 24h add
// from "now".
//
// Ordinary reminders and collections/default notices both use this deadline:
// default catch-up remains eligible after 13:00 on the business day, but the
// delivery budget still ends at the next local midnight. Changing that would
// alter default catch-up semantics and is intentionally out of scope.
func DeliveryDeadlineForBusinessDate(businessDate time.Time) time.Time {
	loc := businessDate.Location()
	if loc == nil {
		loc = time.UTC
	}

	local := businessDate.In(loc)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)

	return dayStart.AddDate(0, 0, 1).UTC()
}
