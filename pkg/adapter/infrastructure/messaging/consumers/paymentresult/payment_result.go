package paymentresult

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// SucceededStatus is a successful Financial Core payment status.
	SucceededStatus = "succeeded"

	// FailedStatus is a failed Financial Core payment status.
	FailedStatus = "failed"

	// SourceTypeBNPLContract identifies BNPL contract checkout payments.
	SourceTypeBNPLContract = "bnpl_contract"
)

// FinancialCorePaymentResultMessage is a local adapter DTO for Financial Core payment result events.
//
// TODO: replace with the official generated Financial Core Kafka event contract when available.
type FinancialCorePaymentResultMessage struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	EventVersion string    `json:"event_version"`
	OccurredAt   time.Time `json:"occurred_at"`

	SourceService string `json:"source_service"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id"`

	PaymentPurpose model.FinancialCorePaymentPurpose `json:"payment_purpose"`

	CorrelationID *string `json:"correlation_id"`
	CausationID   *string `json:"causation_id"`

	ContractID    *model.ContractID    `json:"contract_id"`
	InstallmentID *model.InstallmentID `json:"installment_id"`

	InvoiceID        string `json:"invoice_id"`
	PaymentIntentID  string `json:"payment_intent_id"`
	PaymentAttemptID string `json:"payment_attempt_id"`
	PaymentSessionID string `json:"payment_session_id"`

	Status string `json:"status"`

	PaidAmountMinor uint64    `json:"paid_amount_minor"`
	Currency        string    `json:"currency"`
	PaidAt          time.Time `json:"paid_at"`

	FailureCode *string `json:"failure_code"`
}

// MapPaymentResultStatus maps an external Financial Core payment status to the typed BNPL status.
func MapPaymentResultStatus(raw string) (model.FinancialPaymentResultStatus, error) {
	const op = "paymentresult.MapPaymentResultStatus"

	switch strings.ToLower(strings.TrimSpace(raw)) {
	case SucceededStatus:
		return model.SucceededFinancialPaymentResultStatus, nil

	case FailedStatus:
		return model.FailedFinancialPaymentResultStatus, nil

	case "canceled":
		return model.CanceledFinancialPaymentResultStatus, nil

	default:
		return "", fmt.Errorf("%s: unsupported payment result status %q", op, raw)
	}
}

// IsActivationWorthy reports whether the payment result should activate a BNPL contract.
func (m FinancialCorePaymentResultMessage) IsActivationWorthy() bool {
	return strings.EqualFold(strings.TrimSpace(m.Status), SucceededStatus)
}

// IsInstallmentRepaymentWorthy reports whether the payment result should be applied to an installment.
func (m FinancialCorePaymentResultMessage) IsInstallmentRepaymentWorthy() bool {
	status := strings.ToLower(strings.TrimSpace(m.Status))
	return status == SucceededStatus || status == FailedStatus
}

// ResolvedPaymentPurpose returns the explicit purpose or a legacy checkout default.
func (m FinancialCorePaymentResultMessage) ResolvedPaymentPurpose() model.FinancialCorePaymentPurpose {
	if m.PaymentPurpose != "" {
		return m.PaymentPurpose
	}

	if strings.EqualFold(strings.TrimSpace(m.SourceType), SourceTypeBNPLContract) {
		return model.CheckoutFirstInstallmentFinancialCorePaymentPurpose
	}

	return ""
}

// DecodePaymentResult decodes a Financial Core payment result Kafka payload.
//
// Only the minimum trusted integration envelope is validated here. Payment business fields
// are validated inside financialcorepaymentresultuc after durable inbox acquisition.
func DecodePaymentResult(data []byte) (FinancialCorePaymentResultMessage, error) {
	const op = "paymentresult.DecodePaymentResult"

	if len(data) == 0 {
		return FinancialCorePaymentResultMessage{}, fmt.Errorf("%s: payment result payload is empty", op)
	}

	var message FinancialCorePaymentResultMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return FinancialCorePaymentResultMessage{}, fmt.Errorf("%s: decode payment result payload: %w", op, err)
	}

	if strings.TrimSpace(message.EventID) == "" {
		return FinancialCorePaymentResultMessage{}, fmt.Errorf("%s: payment result event_id is required", op)
	}

	return message, nil
}
