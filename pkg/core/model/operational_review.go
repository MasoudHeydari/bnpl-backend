package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// OperationalReviewEvidence is the durable FC investigation payload BNPL must
// retain when an operation stays Unknown. Fields match the FC operational
// review contract for first-pass triage.
type OperationalReviewEvidence struct {
	OperationName  string `json:"operation_name"`
	SourceType     string `json:"source_type"`
	SourceID       string `json:"source_id"`
	SourceVersion  *int64 `json:"source_version,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	MerchantID     string `json:"merchant_id,omitempty"`
	CorrelationID  string `json:"correlation_id,omitempty"`
}

// Validate reports whether required operational-review identity fields are set.
func (e OperationalReviewEvidence) Validate() error {
	const op = "model.OperationalReviewEvidence.Validate"

	if strings.TrimSpace(e.OperationName) == "" {
		return fmt.Errorf("%s: operation_name is required", op)
	}

	if strings.TrimSpace(e.SourceType) == "" {
		return fmt.Errorf("%s: source.type is required", op)
	}

	if strings.TrimSpace(e.SourceID) == "" {
		return fmt.Errorf("%s: source.id is required", op)
	}

	if strings.TrimSpace(e.IdempotencyKey) == "" {
		return fmt.Errorf("%s: idempotency_key is required", op)
	}

	return nil
}

// OperationalReviewFinding is a documented Financial Core investigation result
// recorded by an authorized operator. It is not itself monetary truth: payment
// status findings must still be confirmed via GetPaymentStatus before BNPL
// mutates money state.
type OperationalReviewFinding string

const (
	// OperationalReviewFindingPaymentStatusSucceeded means FC confirmed the
	// payment intent succeeded. BNPL must still re-query GetPaymentStatus.
	OperationalReviewFindingPaymentStatusSucceeded OperationalReviewFinding = "payment_status_succeeded"

	// OperationalReviewFindingPaymentStatusFailed means FC confirmed terminal
	// payment failure. BNPL must still re-query GetPaymentStatus.
	OperationalReviewFindingPaymentStatusFailed OperationalReviewFinding = "payment_status_failed"

	// OperationalReviewFindingPaymentStatusCanceled means FC confirmed the
	// payment was canceled. BNPL must still re-query GetPaymentStatus.
	OperationalReviewFindingPaymentStatusCanceled OperationalReviewFinding = "payment_status_canceled"

	// OperationalReviewFindingPaymentStatusExpired means FC confirmed the
	// payment session expired. BNPL must still re-query GetPaymentStatus.
	OperationalReviewFindingPaymentStatusExpired OperationalReviewFinding = "payment_status_expired"

	// OperationalReviewFindingNoFinancialEffectConfirmed means FC ops documented
	// that the referenced operation left no successful financial effect. This
	// alone may authorize Safe Termination; it must not invent payment success.
	OperationalReviewFindingNoFinancialEffectConfirmed OperationalReviewFinding = "no_financial_effect_confirmed"

	// OperationalReviewFindingFinancialEffectConfirmedOpen means FC confirmed a
	// successful monetary effect that BNPL cannot auto-compensate. Safe
	// Termination remains forbidden until a valid Refund/Reverse contract exists.
	OperationalReviewFindingFinancialEffectConfirmedOpen OperationalReviewFinding = "financial_effect_confirmed_open"
)

// IsValid reports whether the finding is known.
func (f OperationalReviewFinding) IsValid() bool {
	switch f {
	case OperationalReviewFindingPaymentStatusSucceeded,
		OperationalReviewFindingPaymentStatusFailed,
		OperationalReviewFindingPaymentStatusCanceled,
		OperationalReviewFindingPaymentStatusExpired,
		OperationalReviewFindingNoFinancialEffectConfirmed,
		OperationalReviewFindingFinancialEffectConfirmedOpen:
		return true

	default:
		return false
	}
}

// RequiresGetPaymentStatusConfirmation reports whether BNPL must re-query FC
// before applying the finding to payment-attempt money state.
func (f OperationalReviewFinding) RequiresGetPaymentStatusConfirmation() bool {
	switch f {
	case OperationalReviewFindingPaymentStatusSucceeded,
		OperationalReviewFindingPaymentStatusFailed,
		OperationalReviewFindingPaymentStatusCanceled,
		OperationalReviewFindingPaymentStatusExpired:
		return true

	default:
		return false
	}
}

// AllowsSafeTermination reports whether the finding may authorize local checkout
// / contract termination and credit release after evidence is stored.
func (f OperationalReviewFinding) AllowsSafeTermination() bool {
	return f == OperationalReviewFindingNoFinancialEffectConfirmed
}

// OperationalReviewCaseEnvelope is the durable JSON shape stored on payment
// attempts (last_reconciliation_error) and saga last_error for extractability.
type OperationalReviewCaseEnvelope struct {
	SchemaVersion int                          `json:"schema_version"`
	Kind          string                       `json:"kind"`
	OpenedAt      time.Time                    `json:"opened_at"`
	Evidence      OperationalReviewEvidence    `json:"evidence"`
	OpenReason    string                       `json:"open_reason,omitempty"`
	Resolution    *OperationalReviewResolution `json:"resolution,omitempty"`
}

const (
	operationalReviewEnvelopeKind  = "bnpl.operational_review.v1"
	operationalReviewSchemaVersion = 1
)

// NewOperationalReviewCaseEnvelope builds an open case envelope.
func NewOperationalReviewCaseEnvelope(
	evidence OperationalReviewEvidence,
	openReason string,
	openedAt time.Time,
) (OperationalReviewCaseEnvelope, error) {
	const op = "model.NewOperationalReviewCaseEnvelope"

	if err := evidence.Validate(); err != nil {
		return OperationalReviewCaseEnvelope{}, fmt.Errorf("%s: %w", op, err)
	}

	return OperationalReviewCaseEnvelope{
		SchemaVersion: operationalReviewSchemaVersion,
		Kind:          operationalReviewEnvelopeKind,
		OpenedAt:      openedAt.UTC(),
		Evidence:      evidence,
		OpenReason:    strings.TrimSpace(openReason),
	}, nil
}

// MarshalOperationalReviewCase serializes an open or resolved case for durable storage.
func MarshalOperationalReviewCase(envelope OperationalReviewCaseEnvelope) (string, error) {
	const op = "model.MarshalOperationalReviewCase"

	if envelope.Kind != operationalReviewEnvelopeKind {
		return "", fmt.Errorf("%s: unsupported envelope kind %q", op, envelope.Kind)
	}

	if err := envelope.Evidence.Validate(); err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("%s: marshal: %w", op, err)
	}

	return string(raw), nil
}

// ParseOperationalReviewCase extracts a case envelope from stored text.
func ParseOperationalReviewCase(raw string) (OperationalReviewCaseEnvelope, bool, error) {
	const op = "model.ParseOperationalReviewCase"

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.Contains(trimmed, operationalReviewEnvelopeKind) {
		return OperationalReviewCaseEnvelope{}, false, nil
	}

	var envelope OperationalReviewCaseEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return OperationalReviewCaseEnvelope{}, false, fmt.Errorf("%s: unmarshal: %w", op, err)
	}

	if envelope.Kind != operationalReviewEnvelopeKind {
		return OperationalReviewCaseEnvelope{}, false, nil
	}

	if err := envelope.Evidence.Validate(); err != nil {
		return OperationalReviewCaseEnvelope{}, true, fmt.Errorf("%s: %w", op, err)
	}

	return envelope, true, nil
}

// OperationalReviewResolution is the durable operator resolution payload.
type OperationalReviewResolution struct {
	Finding           OperationalReviewFinding `json:"finding"`
	ResolutionKey     string                   `json:"resolution_idempotency_key"`
	FCReference       string                   `json:"fc_reference"`
	OperatorSubjectID string                   `json:"operator_subject_id,omitempty"`
	Notes             string                   `json:"notes,omitempty"`
	ResolvedAt        time.Time                `json:"resolved_at"`
}

// Validate reports whether a resolution carries required FC documentation.
func (r OperationalReviewResolution) Validate() error {
	const op = "model.OperationalReviewResolution.Validate"

	if !r.Finding.IsValid() {
		return fmt.Errorf("%s: finding is invalid", op)
	}

	if strings.TrimSpace(r.ResolutionKey) == "" {
		return fmt.Errorf("%s: resolution_idempotency_key is required", op)
	}

	if strings.TrimSpace(r.FCReference) == "" {
		return fmt.Errorf("%s: fc_reference is required", op)
	}

	if r.ResolvedAt.IsZero() {
		return fmt.Errorf("%s: resolved_at is required", op)
	}

	return nil
}
