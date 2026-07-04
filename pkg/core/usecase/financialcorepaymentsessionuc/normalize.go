package financialcorepaymentsessionuc

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	installmentPaymentSessionCreatedEventType = "financial_core.installment_payment_session_created.v1"
	expectedEventVersion                      = "1"
	expectedSourceService                     = "financial-core"
)

// ProcessInput carries a Financial Core installment payment-session-created event.
type ProcessInput struct {
	// EventID uniquely identifies the inbound event.
	EventID string
	// EventType is the inbound event type.
	EventType string
	// EventVersion is the inbound event schema version.
	EventVersion string
	// SourceService identifies the producing service.
	SourceService string

	// CorrelationID correlates related events across services.
	CorrelationID *string
	// CausationID identifies the causing event or request.
	CausationID *string

	// RawPayload is the original event payload bytes.
	RawPayload json.RawMessage

	// BNPLPaymentAttemptID identifies the BNPL payment attempt.
	BNPLPaymentAttemptID model.PaymentAttemptID
	// FinancialPaymentIntentID identifies the Financial Core payment intent.
	FinancialPaymentIntentID string
	// FinancialPaymentAttemptID identifies the Financial Core payment attempt.
	FinancialPaymentAttemptID *string
	// FinancialPaymentSessionID identifies the Financial Core payment session.
	FinancialPaymentSessionID string
	// PaymentURL is the hosted payment URL for the customer.
	PaymentURL string
	// ExpiresAt is when the offer, session, or limit expires.
	ExpiresAt time.Time
	// AmountMinor is the amount in minor currency units.
	AmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// EventOccurredAt is when the source event occurred.
	EventOccurredAt time.Time
}

// normalizeProcessInput trims integration identity fields once so the same values are used
// for inbox persistence, fingerprinting, conflict detection, and business validation.
func normalizeProcessInput(input ProcessInput) ProcessInput {
	input.EventID = strings.TrimSpace(input.EventID)
	input.EventType = strings.TrimSpace(input.EventType)
	input.EventVersion = strings.TrimSpace(input.EventVersion)
	input.SourceService = strings.TrimSpace(input.SourceService)
	input.FinancialPaymentIntentID = strings.TrimSpace(input.FinancialPaymentIntentID)
	input.FinancialPaymentSessionID = strings.TrimSpace(input.FinancialPaymentSessionID)
	input.PaymentURL = strings.TrimSpace(input.PaymentURL)
	input.Currency = model.Currency(strings.TrimSpace(input.Currency.String()))

	if input.FinancialPaymentAttemptID != nil {
		value := strings.TrimSpace(*input.FinancialPaymentAttemptID)
		if value == "" {
			input.FinancialPaymentAttemptID = nil
		} else {
			input.FinancialPaymentAttemptID = &value
		}
	}

	if input.CorrelationID != nil {
		value := strings.TrimSpace(*input.CorrelationID)
		input.CorrelationID = &value
	}

	if input.CausationID != nil {
		value := strings.TrimSpace(*input.CausationID)
		input.CausationID = &value
	}

	return input
}
