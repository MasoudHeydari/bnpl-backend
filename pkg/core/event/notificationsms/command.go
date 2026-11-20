package notificationsms

import (
	"fmt"
	"strings"
	"time"
)

const (
	// SchemaVersionCommand is the Notification Service command envelope version
	// consumed from topic notification.commands.v1.
	SchemaVersionCommand = 1

	// EventTypeCommand is the Outbox event_type for Notification command
	// publishes. It is distinct from notification_type inside the JSON body.
	EventTypeCommand = "notification.command.v1"

	// CallerHeaderKey is the Kafka record header Notification requires.
	CallerHeaderKey = "x-notification-caller"

	// CallerHeaderValue identifies BNPL as the Kafka caller.
	CallerHeaderValue = ProducerName

	// DefaultPriority is the transactional command priority for reminders.
	DefaultPriority = 5

	// SelectionSourceUpstreamSelected means BNPL chose the delivery template.
	SelectionSourceUpstreamSelected = "upstream_selected"

	// BusinessReferenceTypeBNPLInstallment is Notification's installment ref kind.
	BusinessReferenceTypeBNPLInstallment = "BNPL_INSTALLMENT"
)

// Command is the Notification Service Kafka command envelope for repayment
// reminder and collections default SMS requests on notification.commands.v1.
//
// customer_id on each target must be the CRS external customer id, never
// customer_profiles.id and never an Identity subject ID. Phone numbers must
// not appear. payment_url must be an absolute https URL.
type Command struct {
	SchemaVersion    int             `json:"schema_version"`
	SourceService    string          `json:"source_service"`
	SourceEventID    string          `json:"source_event_id"`
	IdempotencyKey   string          `json:"idempotency_key"`
	NotificationType string          `json:"notification_type"`
	Category         string          `json:"category"`
	Priority         int             `json:"priority"`
	ExpiresAt        time.Time       `json:"expires_at"`
	Targets          []CommandTarget `json:"targets"`
}

// CommandTarget identifies one notification target and its deliveries.
type CommandTarget struct {
	TargetKey             string            `json:"target_key"`
	TargetType            string            `json:"target_type"`
	ExternalCustomerID    string            `json:"customer_id"`
	BusinessReferenceType string            `json:"business_reference_type"`
	BusinessReferenceID   string            `json:"business_reference_id"`
	Deliveries            []CommandDelivery `json:"deliveries"`
}

// CommandDelivery describes one SMS delivery specification.
type CommandDelivery struct {
	DeliveryMethod  string            `json:"delivery_method"`
	Channel         string            `json:"channel"`
	SelectionSource string            `json:"selection_source"`
	TemplateCode    string            `json:"template_code"`
	Variables       map[string]string `json:"variables"`
}

// CommandInput carries durable fields required to construct Command once.
type CommandInput struct {
	SourceEventID      string
	IdempotencyKey     string
	NotificationType   string
	ExternalCustomerID string
	InstallmentID      string
	PaymentURL         string
	ExpiresAt          time.Time
}

// NewCommand constructs a Notification commands.v1 envelope.
func NewCommand(input CommandInput) Command {
	vars := map[string]string{
		"payment_url": input.PaymentURL,
	}

	return Command{
		SchemaVersion:    SchemaVersionCommand,
		SourceService:    ProducerName,
		SourceEventID:    input.SourceEventID,
		IdempotencyKey:   input.IdempotencyKey,
		NotificationType: input.NotificationType,
		Category:         CategoryTransactional,
		Priority:         DefaultPriority,
		ExpiresAt:        input.ExpiresAt.UTC(),
		Targets: []CommandTarget{{
			TargetKey:             TargetKeyCustomer,
			TargetType:            TargetTypeCustomer,
			ExternalCustomerID:    input.ExternalCustomerID,
			BusinessReferenceType: BusinessReferenceTypeBNPLInstallment,
			BusinessReferenceID:   input.InstallmentID,
			Deliveries: []CommandDelivery{{
				DeliveryMethod:  DeliveryMethodSMS,
				Channel:         ChannelSMS,
				SelectionSource: SelectionSourceUpstreamSelected,
				TemplateCode:    input.NotificationType,
				Variables:       vars,
			}},
		}},
	}
}

// ValidateCommand validates a commands.v1 envelope before outbox insertion.
func ValidateCommand(event Command) error {
	const op = "notificationsms.ValidateCommand"

	if event.SchemaVersion != SchemaVersionCommand {
		return fmt.Errorf("%s: schema_version must be %d", op, SchemaVersionCommand)
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

	if event.Priority != DefaultPriority {
		return fmt.Errorf("%s: priority must be %d", op, DefaultPriority)
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

	if target.BusinessReferenceType != BusinessReferenceTypeBNPLInstallment {
		return fmt.Errorf("%s: business_reference_type must be %q", op, BusinessReferenceTypeBNPLInstallment)
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

	if delivery.SelectionSource != SelectionSourceUpstreamSelected {
		return fmt.Errorf("%s: selection_source must be %q", op, SelectionSourceUpstreamSelected)
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

	paymentURL := strings.TrimSpace(delivery.Variables["payment_url"])
	if paymentURL == "" {
		return fmt.Errorf("%s: payment_url variable is required", op)
	}

	if !strings.HasPrefix(paymentURL, "https://") {
		return fmt.Errorf("%s: payment_url must be an absolute https URL", op)
	}

	if _, hasPhone := delivery.Variables["recipient_address"]; hasPhone {
		return fmt.Errorf("%s: recipient_address must not be present", op)
	}

	if _, hasHash := delivery.Variables["recipient_hash"]; hasHash {
		return fmt.Errorf("%s: recipient_hash must not be present", op)
	}

	return nil
}

// IsCommandsTopic reports whether destination is Notification's command topic.
func IsCommandsTopic(destination string) bool {
	return strings.TrimSpace(destination) == "notification.commands.v1" ||
		strings.HasSuffix(strings.TrimSpace(destination), ".commands.v1") ||
		strings.Contains(strings.TrimSpace(destination), "notification.commands")
}
