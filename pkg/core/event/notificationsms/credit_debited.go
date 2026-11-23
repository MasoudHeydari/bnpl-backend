package notificationsms

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// NotificationTypeCreditMonthlyDebited is the pay-next-month credit debit
	// SMS notification_type and template_code (must match Notification catalog).
	NotificationTypeCreditMonthlyDebited = "bnpl.credit.pay_next_month.debited.v1"

	// NotificationTypeCreditInstallmentDebited is the pay-in-installments credit
	// debit SMS notification_type and template_code (must match Notification
	// catalog). Legacy bnpl.credit.installment.debited.v1 is no longer registered.
	NotificationTypeCreditInstallmentDebited = "bnpl.credit.pay_in_installments.debited.v1"

	// BusinessReferenceTypeBNPLPurchase is Notification's purchase reference kind.
	BusinessReferenceTypeBNPLPurchase = "BNPL_PURCHASE"

	// BusinessReferenceCheckoutSessionPrefix prefixes internal checkout_sessions.id.
	BusinessReferenceCheckoutSessionPrefix = "cs_"

	// CreditDebitedIdempotencyKeyPrefix prefixes the durable credit-transaction id.
	CreditDebitedIdempotencyKeyPrefix = "bnpl.credit.debited:"

	// CreditDebitedExpiryWindow is the maximum delivery window after debit.
	CreditDebitedExpiryWindow = 24 * time.Hour

	creditDebitedVarDebitedAmount            = "debited_amount"
	creditDebitedVarRemainingAvailableCredit = "remaining_available_credit"
	creditDebitedVarCurrency                 = "currency"
	creditDebitedCurrencyIRR                  = "IRR"
	creditDebitedCurrencyTOMAN                = "TOMAN"
)

// CreditDebitedCommandInput carries durable fields for a credit-debited command.
type CreditDebitedCommandInput struct {
	SourceEventID             string
	IdempotencyKey            string
	NotificationType          string
	ExternalCustomerID        string
	CheckoutSessionID         string
	DebitedAmountMinor        uint64
	RemainingAvailableCredit  uint64
	Currency                  string
	ExpiresAt                 time.Time
}

// CreditDebitedIdempotencyKey builds the stable Notification idempotency key
// from the credit_transactions.id that recorded the exposure increase.
func CreditDebitedIdempotencyKey(creditTransactionID string) string {
	return CreditDebitedIdempotencyKeyPrefix + strings.TrimSpace(creditTransactionID)
}

// FormatCheckoutSessionBusinessReferenceID builds BNPL_PURCHASE business_reference_id.
func FormatCheckoutSessionBusinessReferenceID(checkoutSessionID string) string {
	return BusinessReferenceCheckoutSessionPrefix + strings.TrimSpace(checkoutSessionID)
}

// NewCreditDebitedCommand constructs a Notification commands.v1 envelope for a
// definitive BNPL credit debit (exposure increase after contract activation).
func NewCreditDebitedCommand(input CreditDebitedCommandInput) Command {
	vars := map[string]string{
		creditDebitedVarDebitedAmount:            strconv.FormatUint(input.DebitedAmountMinor, 10),
		creditDebitedVarRemainingAvailableCredit: strconv.FormatUint(input.RemainingAvailableCredit, 10),
		creditDebitedVarCurrency:                 strings.TrimSpace(input.Currency),
	}

	return Command{
		SchemaVersion:    SchemaVersionCommand,
		SourceService:    ProducerName,
		SourceEventID:    strings.TrimSpace(input.SourceEventID),
		IdempotencyKey:   strings.TrimSpace(input.IdempotencyKey),
		NotificationType: strings.TrimSpace(input.NotificationType),
		Category:         CategoryTransactional,
		Priority:         DefaultPriority,
		ExpiresAt:        input.ExpiresAt.UTC(),
		Targets: []CommandTarget{{
			TargetKey:             TargetKeyCustomer,
			TargetType:            TargetTypeCustomer,
			ExternalCustomerID:    strings.TrimSpace(input.ExternalCustomerID),
			BusinessReferenceType: BusinessReferenceTypeBNPLPurchase,
			BusinessReferenceID:   FormatCheckoutSessionBusinessReferenceID(input.CheckoutSessionID),
			Deliveries: []CommandDelivery{{
				DeliveryMethod:  DeliveryMethodSMS,
				Channel:         ChannelSMS,
				SelectionSource: SelectionSourceUpstreamSelected,
				TemplateCode:    strings.TrimSpace(input.NotificationType),
				Variables:       vars,
			}},
		}},
	}
}

// ValidateCreditDebitedCommand validates a credit-debited commands.v1 envelope.
// It must not require payment_url; that variable is reminder-only.
func ValidateCreditDebitedCommand(event Command) error {
	const op = "notificationsms.ValidateCreditDebitedCommand"

	if event.SchemaVersion != SchemaVersionCommand {
		return fmt.Errorf("%s: schema_version must be %d", op, SchemaVersionCommand)
	}

	if strings.TrimSpace(event.SourceService) != ProducerName {
		return fmt.Errorf("%s: source_service must be %q", op, ProducerName)
	}

	if err := requireUUID(op, "source_event_id", event.SourceEventID); err != nil {
		return err
	}

	idempotencyKey := strings.TrimSpace(event.IdempotencyKey)
	if idempotencyKey == "" {
		return fmt.Errorf("%s: idempotency_key is required", op)
	}

	expectedKey := CreditDebitedIdempotencyKey(event.SourceEventID)
	if idempotencyKey != expectedKey {
		return fmt.Errorf("%s: idempotency_key must be %q", op, expectedKey)
	}

	if !isCreditDebitedNotificationType(event.NotificationType) {
		return fmt.Errorf("%s: notification_type must be a credit-debited template", op)
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

	if event.ExpiresAt.Location() != time.UTC {
		return fmt.Errorf("%s: expires_at must be UTC", op)
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

	if err := requireUUID(op, "customer_id", target.ExternalCustomerID); err != nil {
		return err
	}

	if target.BusinessReferenceType != BusinessReferenceTypeBNPLPurchase {
		return fmt.Errorf("%s: business_reference_type must be %q", op, BusinessReferenceTypeBNPLPurchase)
	}

	businessReferenceID := strings.TrimSpace(target.BusinessReferenceID)
	if !strings.HasPrefix(businessReferenceID, BusinessReferenceCheckoutSessionPrefix) {
		return fmt.Errorf("%s: business_reference_id must start with %q", op, BusinessReferenceCheckoutSessionPrefix)
	}

	sessionID := strings.TrimPrefix(businessReferenceID, BusinessReferenceCheckoutSessionPrefix)
	if err := requireUUID(op, "business_reference_id checkout session id", sessionID); err != nil {
		return err
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

	if _, hasPaymentURL := delivery.Variables["payment_url"]; hasPaymentURL {
		return fmt.Errorf("%s: payment_url must not be present", op)
	}

	if _, hasPhone := delivery.Variables["recipient_address"]; hasPhone {
		return fmt.Errorf("%s: recipient_address must not be present", op)
	}

	if _, hasHash := delivery.Variables["recipient_hash"]; hasHash {
		return fmt.Errorf("%s: recipient_hash must not be present", op)
	}

	if _, hasPhone := delivery.Variables["phone"]; hasPhone {
		return fmt.Errorf("%s: phone must not be present", op)
	}

	if _, hasMessage := delivery.Variables["message"]; hasMessage {
		return fmt.Errorf("%s: message must not be present", op)
	}

	if len(delivery.Variables) != 3 {
		return fmt.Errorf("%s: variables must contain exactly debited_amount, remaining_available_credit, and currency", op)
	}

	debitedRaw, ok := delivery.Variables[creditDebitedVarDebitedAmount]
	if !ok {
		return fmt.Errorf("%s: debited_amount variable is required", op)
	}

	debited, err := parseStrictUint(op, creditDebitedVarDebitedAmount, debitedRaw)
	if err != nil {
		return err
	}

	if debited == 0 {
		return fmt.Errorf("%s: debited_amount must be positive", op)
	}

	remainingRaw, ok := delivery.Variables[creditDebitedVarRemainingAvailableCredit]
	if !ok {
		return fmt.Errorf("%s: remaining_available_credit variable is required", op)
	}

	if _, err = parseStrictUint(op, creditDebitedVarRemainingAvailableCredit, remainingRaw); err != nil {
		return err
	}

	currency := strings.TrimSpace(delivery.Variables[creditDebitedVarCurrency])
	if currency != creditDebitedCurrencyIRR && currency != creditDebitedCurrencyTOMAN {
		return fmt.Errorf("%s: currency must be %q or %q", op, creditDebitedCurrencyIRR, creditDebitedCurrencyTOMAN)
	}

	return nil
}

func isCreditDebitedNotificationType(notificationType string) bool {
	switch strings.TrimSpace(notificationType) {
	case NotificationTypeCreditMonthlyDebited, NotificationTypeCreditInstallmentDebited:
		return true

	default:
		return false
	}
}

func requireUUID(op, field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s: %s is required", op, field)
	}

	if _, err := uuid.Parse(trimmed); err != nil {
		return fmt.Errorf("%s: %s must be a valid UUID", op, field)
	}

	return nil
}

func parseStrictUint(op, field, raw string) (uint64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("%s: %s is required", op, field)
	}

	if strings.ContainsAny(trimmed, "., ") || strings.HasPrefix(trimmed, "-") {
		return 0, fmt.Errorf("%s: %s must be a non-negative integer string", op, field)
	}

	value, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %s must be a non-negative integer string", op, field)
	}

	if strconv.FormatUint(value, 10) != trimmed {
		return 0, fmt.Errorf("%s: %s must be a canonical integer string", op, field)
	}

	return value, nil
}
