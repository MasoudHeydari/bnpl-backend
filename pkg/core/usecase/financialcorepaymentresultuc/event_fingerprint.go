package financialcorepaymentresultuc

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const financialCorePaymentResultFingerprintVersion = "financial-core-payment-result:v1:"

type canonicalPaymentResultFingerprint struct {
	SourceService             string `json:"source_service"`
	EventID                   string `json:"event_id"`
	EventType                 string `json:"event_type"`
	EventVersion              string `json:"event_version"`
	PaymentPurpose            string `json:"payment_purpose"`
	SourceType                string `json:"source_type,omitempty"`
	SourceID                  string `json:"source_id,omitempty"`
	InvoiceID                 string `json:"invoice_id,omitempty"`
	FinancialPaymentIntentID  string `json:"financial_payment_intent_id"`
	FinancialPaymentAttemptID string `json:"financial_payment_attempt_id"`
	PaymentSessionID          string `json:"payment_session_id,omitempty"`
	ContractID                string `json:"contract_id,omitempty"`
	InstallmentID             string `json:"installment_id,omitempty"`
	Status                    string `json:"status"`
	AmountMinor               uint64 `json:"amount_minor"`
	Currency                  string `json:"currency"`
	PaidAt                    string `json:"paid_at,omitempty"`
	EventOccurredAt           string `json:"event_occurred_at,omitempty"`
	FailureCode               string `json:"failure_code,omitempty"`
}

func (uc *UseCase) paymentResultPayloadFingerprint(
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
) (string, error) {
	const op = "financialcorepaymentresultuc.UseCase.paymentResultPayloadFingerprint"

	status := canonicalPaymentStatus(input)

	canonical := canonicalPaymentResultFingerprint{
		SourceService:             input.SourceService,
		EventID:                   input.EventID,
		EventType:                 input.EventType,
		EventVersion:              input.EventVersion,
		PaymentPurpose:            string(purpose),
		SourceType:                input.SourceType,
		SourceID:                  input.SourceID,
		InvoiceID:                 input.InvoiceID,
		FinancialPaymentIntentID:  input.PaymentIntentID,
		FinancialPaymentAttemptID: input.PaymentAttemptID,
		PaymentSessionID:          input.PaymentSessionID,
		Status:                    status,
		AmountMinor:               input.PaidAmountMinor,
		Currency:                  input.Currency.String(),
	}

	if !input.ContractID.IsZero() {
		canonical.ContractID = input.ContractID.String()
	}

	if !input.InstallmentID.IsZero() {
		canonical.InstallmentID = input.InstallmentID.String()
	}

	if !input.PaidAt.IsZero() {
		canonical.PaidAt = input.PaidAt.UTC().Format(time.RFC3339Nano)
	}

	if !input.EventOccurredAt.IsZero() {
		canonical.EventOccurredAt = input.EventOccurredAt.UTC().Format(time.RFC3339Nano)
	}

	if input.FailureCode != nil {
		canonical.FailureCode = *input.FailureCode
	}

	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("%s: marshal canonical payment result fingerprint: %w", op, err)
	}

	canonicalData := append([]byte(financialCorePaymentResultFingerprintVersion), raw...)

	value, err := uc.fingerprintHasher.Hash(string(canonicalData))
	if err != nil {
		return "", fmt.Errorf("%s: hash fingerprint: %w", op, err)
	}

	if err := value.Validate(); err != nil {
		return "", fmt.Errorf("%s: validate fingerprint: %w", op, err)
	}

	return value.String(), nil
}
