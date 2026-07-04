package paymentreconciliation

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ResultMessage is a local adapter DTO for Financial Core reconciliation result events.
type ResultMessage struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	EventVersion string    `json:"event_version"`
	OccurredAt   time.Time `json:"occurred_at"`

	SourceService string `json:"source_service"`

	CorrelationID *string `json:"correlation_id"`
	CausationID   *string `json:"causation_id"`

	Data ResultData `json:"data"`
}

// ResultData carries the reconciliation result business payload.
type ResultData struct {
	RequestEventID            string                 `json:"request_event_id"`
	BNPLPaymentAttemptID      model.PaymentAttemptID `json:"bnpl_payment_attempt_id"`
	ReconciliationGeneration  int                    `json:"reconciliation_generation"`
	Status                    string                 `json:"status"`
	FinancialPaymentIntentID  *string                `json:"financial_payment_intent_id"`
	FinancialPaymentAttemptID *string                `json:"financial_payment_attempt_id"`
	FinancialPaymentSessionID *string                `json:"financial_payment_session_id"`
	PaymentURL                *string                `json:"payment_url"`
	ExpiresAt                 *time.Time             `json:"expires_at"`
	PaidAt                    *time.Time             `json:"paid_at"`
	FailureCode               *string                `json:"failure_code"`
	AmountMinor               uint64                 `json:"amount_minor"`
	Currency                  string                 `json:"currency"`
	OccurredAt                time.Time              `json:"occurred_at"`
}

// DecodeResult unmarshals a Financial Core reconciliation result message.
func DecodeResult(raw []byte) (ResultMessage, error) {
	var message ResultMessage
	const op = "paymentreconciliation.DecodeResult"

	if err := json.Unmarshal(raw, &message); err != nil {
		return ResultMessage{}, fmt.Errorf("%s: unmarshal reconciliation result: %w", op, err)
	}

	return message, nil
}
