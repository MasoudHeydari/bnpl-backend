package paymentsession

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// InstallmentPaymentSessionCreatedMessage is a local adapter DTO for Financial Core
// installment payment-session-created events.
//
// TODO: replace with the official generated Financial Core Kafka event contract when available.
type InstallmentPaymentSessionCreatedMessage struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	EventVersion string    `json:"event_version"`
	OccurredAt   time.Time `json:"occurred_at"`

	SourceService string `json:"source_service"`

	CorrelationID *string `json:"correlation_id"`
	CausationID   *string `json:"causation_id"`

	Data InstallmentPaymentSessionCreatedData `json:"data"`
}

// InstallmentPaymentSessionCreatedData carries the payment-session-created business payload.
type InstallmentPaymentSessionCreatedData struct {
	BNPLPaymentAttemptID      model.PaymentAttemptID `json:"bnpl_payment_attempt_id"`
	FinancialPaymentIntentID  string                 `json:"financial_payment_intent_id"`
	FinancialPaymentAttemptID *string                `json:"financial_payment_attempt_id"`
	FinancialPaymentSessionID string                 `json:"financial_payment_session_id"`
	PaymentURL                string                 `json:"payment_url"`
	ExpiresAt                 time.Time              `json:"expires_at"`
	AmountMinor               uint64                 `json:"amount_minor"`
	Currency                  string                 `json:"currency"`
	OccurredAt                time.Time              `json:"occurred_at"`
}

// DecodeInstallmentPaymentSessionCreated decodes a Financial Core payment-session-created payload.
func DecodeInstallmentPaymentSessionCreated(data []byte) (InstallmentPaymentSessionCreatedMessage, error) {
	const op = "paymentsession.DecodeInstallmentPaymentSessionCreated"

	if len(data) == 0 {
		return InstallmentPaymentSessionCreatedMessage{}, fmt.Errorf("%s: payment session payload is empty", op)
	}

	var message InstallmentPaymentSessionCreatedMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return InstallmentPaymentSessionCreatedMessage{}, fmt.Errorf("%s: decode payment session payload: %w", op, err)
	}

	if strings.TrimSpace(message.EventID) == "" {
		return InstallmentPaymentSessionCreatedMessage{}, fmt.Errorf("%s: payment session event_id is required", op)
	}

	return message, nil
}
