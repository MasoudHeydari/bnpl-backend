package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Audit is emitted only for newly applied durable business mutations.
// Duplicate Inbox deliveries do not produce duplicate Audit Events: the Inbox
// acquire result is the idempotency guard. Financial Core is the Audit Actor.
// The inbound Financial Core EventID is the direct causation_event_id.
// Raw Kafka payloads are intentionally excluded from Metadata (Inbox preserves them).
// Audit and payment-application result events are separate Outbox records with
// distinct destinations, event types, and message keys.

const (
	auditActionContractActivateFromCheckoutPayment        = "contract.activate_from_checkout_payment"
	auditActionInstallmentApplyRepayment                  = "installment.apply_repayment"
	auditActionPaymentAttemptMarkFailed                   = "payment_attempt.mark_failed"
	auditActionPaymentAttemptMarkCanceled                 = "payment_attempt.mark_canceled"
	auditActionPaymentResultRequireReconciliation         = "payment_result.require_reconciliation"
	auditActionPaymentResultRecordUnmatchedTerminalResult = "payment_result.record_unmatched_terminal_result"

	auditReasonCheckoutFirstInstallmentPaymentSucceeded = "checkout_first_installment_payment_succeeded"
	auditReasonInstallmentRepaymentApplied              = "installment_repayment_applied"
	auditReasonPaymentAttemptCanceled                   = "payment_attempt_canceled"
	auditReasonPayloadIdentityConflict                  = "event_identity_payload_mismatch"

	contractResourceType       = "contract"
	installmentResourceType    = "installment"
	paymentAttemptResourceType = "payment_attempt"
	inboxResourceType          = "inbox"
)

func (uc *UseCase) newAuditEventID() string {
	return uc.idGenerator.Generate()
}

func (uc *UseCase) insertAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	event audit.Event,
	now time.Time,
) error {
	const op = "financialcorepaymentresultuc.UseCase.insertAuditOutbox"

	if uc.auditDestination == "" {
		return fmt.Errorf("%s: audit destination is not configured", op)
	}

	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate payment result audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal payment result audit event: %w", op, err)
	}

	aggregateType := event.Data.Resource.Type
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    event.Data.Resource.ID,
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert payment result audit outbox message: %w", op, err)
	}

	return nil
}

// resolveAuditEnvelopeIDs prefers the inbound event correlation ID, then the
// trusted request-context correlation ID. Trace ID always comes from context.
func resolveAuditEnvelopeIDs(ctx context.Context, input ProcessInput) (correlationID string, traceID string) {
	if input.CorrelationID != nil {
		if value := strings.TrimSpace(*input.CorrelationID); value != "" {
			correlationID = value
		}
	}

	if correlationID == "" {
		correlationID = strings.TrimSpace(trace.CorrelationID(ctx))
	}

	if correlationID == "" {
		correlationID = strings.TrimSpace(input.EventID)
	}

	traceID = strings.TrimSpace(trace.ID(ctx))
	if traceID == "" {
		traceID = correlationID
	}

	return correlationID, traceID
}

func auditActor(sourceService string) audit.Actor {
	service := strings.TrimSpace(sourceService)
	if service == "" {
		service = installmentpaymentreconciliation.FinancialCoreSourceService
	}

	return audit.Actor{
		Type:    audit.ServiceActorType,
		Service: &service,
	}
}

func causationEventID(input ProcessInput) *string {
	eventID := strings.TrimSpace(input.EventID)
	if eventID == "" {
		return nil
	}

	return &eventID
}

func basePaymentResultAuditMetadata(
	input ProcessInput,
	inboxID model.InboxID,
	processResult ProcessResult,
) map[string]any {
	actor := auditActor(input.SourceService)
	metadata := map[string]any{
		"financial_core_event_id":       input.EventID,
		"financial_core_event_type":     input.EventType,
		"financial_core_event_version":  input.EventVersion,
		"financial_core_source_service": *actor.Service,
		"payment_purpose":               input.PaymentPurpose,
		"payment_status":                canonicalPaymentStatus(input),
		"inbox_id":                      inboxID.String(),
		"process_result":                processResult,
	}

	if !input.EventOccurredAt.IsZero() {
		metadata["financial_core_event_occurred_at"] = input.EventOccurredAt.UTC()
	}

	if input.PaymentIntentID != "" {
		metadata["payment_intent_id"] = input.PaymentIntentID
	}

	if input.PaymentAttemptID != "" {
		metadata["financial_core_payment_attempt_id"] = input.PaymentAttemptID
	}

	if input.PaymentSessionID != "" {
		metadata["payment_session_id"] = input.PaymentSessionID
	}

	if input.InvoiceID != "" {
		metadata["invoice_id"] = input.InvoiceID
	}

	if input.SourceType != "" {
		metadata["source_type"] = input.SourceType
	}

	if input.SourceID != "" {
		metadata["source_id"] = input.SourceID
	}

	if input.PaidAmountMinor > 0 {
		metadata["paid_amount_minor"] = input.PaidAmountMinor
	}

	if input.Currency != "" {
		metadata["currency"] = input.Currency
	}

	if !input.PaidAt.IsZero() {
		metadata["paid_at"] = input.PaidAt.UTC()
	}

	if input.CausationID != nil {
		if value := strings.TrimSpace(*input.CausationID); value != "" {
			metadata["upstream_causation_id"] = value
		}
	}

	return metadata
}

type optionalTypedID interface {
	IsZero() bool
	String() string
}

func putOptionalUUID(metadata map[string]any, key string, id optionalTypedID) {
	if id.IsZero() {
		return
	}

	metadata[key] = id.String()
}

func putOptionalUUIDPtr(metadata map[string]any, key string, id *model.MerchantBranchProfileID) {
	if id == nil || id.IsZero() {
		return
	}

	metadata[key] = id.String()
}

func normalizedReasonCodes(codes ...string) []string {
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}

		out = append(out, code)
	}

	return out
}

func primaryReasonCode(codes []string) *string {
	if len(codes) == 0 {
		return nil
	}

	reasonCode := codes[0]
	return &reasonCode
}

func putReasonCodes(metadata map[string]any, codes []string) {
	if len(codes) == 0 {
		return
	}

	metadata["reason_codes"] = append([]string(nil), codes...)
}
