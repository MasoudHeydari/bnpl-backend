package paymentattemptreconciliationuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	auditActionPaymentAttemptRequireReconciliation = "payment_attempt.require_reconciliation"
	auditActionPaymentAttemptRequestReconciliation = "payment_attempt.request_reconciliation"
	auditActionPaymentAttemptRequireManualReview   = "payment_attempt.require_manual_review"

	auditReasonAutomaticReconciliationRequested = "automatic_reconciliation_requested"

	manualReviewRetryBudgetExhausted = "reconciliation_retry_budget_exhausted"
	manualReviewRequestBuildFailed   = "reconciliation_request_build_failed"

	auditServicePaymentAttemptReconciliationTask = "payment-attempt-reconciliation-task"
)

func (uc *UseCase) insertAuditOutbox(ctx context.Context, q repo.OutboxTxQueryer, event audit.Event, now time.Time) error {
	const op = "paymentattemptreconciliationuc.UseCase.insertAuditOutbox"

	if uc.auditDestination == "" {
		return fmt.Errorf("%s: audit destination is not configured", op)
	}

	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate payment attempt reconciliation audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal payment attempt reconciliation audit event: %w", op, err)
	}

	aggregateType := event.Data.Resource.Type
	_, err = q.Create(ctx, repo.CreateOutboxMessage{
		Destination: uc.auditDestination, MessageKey: event.Data.Resource.ID,
		EventType: audit.EventType, AggregateType: &aggregateType, Payload: payload, CreatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert payment attempt reconciliation audit outbox message: %w", op, err)
	}

	return nil
}

func (uc *UseCase) newRequireReconciliationAuditEvent(before model.PaymentAttemptState, attempt model.PaymentAttempt, now time.Time) audit.Event {
	reason := "reconciliation_required"
	if attempt.ReconciliationReason != nil {
		reason = string(*attempt.ReconciliationReason)
	}

	return newPaymentAttemptReconciliationAuditEvent(
		uc.idGenerator.Generate(),
		auditActionPaymentAttemptRequireReconciliation,
		reason,
		audit.SucceededOutcome,
		attempt,
		now,
		[]audit.FieldChange{{Field: "payment_attempt.state", Before: before, After: attempt.State}},
		map[string]any{"next_reconciliation_at": optionalTime(attempt.NextReconciliationAt)},
	)
}

func (uc *UseCase) newRequestReconciliationAuditEvent(before, after model.PaymentAttempt, requestEventID string, now time.Time) audit.Event {
	metadata := map[string]any{
		"reconciliation_request_event_id": requestEventID,
		"reconciliation_requested_at":     optionalTime(after.ReconciliationRequestedAt),
		"next_reconciliation_at":          optionalTime(after.NextReconciliationAt),
	}
	return newPaymentAttemptReconciliationAuditEvent(
		uc.idGenerator.Generate(),
		auditActionPaymentAttemptRequestReconciliation,
		auditReasonAutomaticReconciliationRequested,
		audit.SucceededOutcome,
		after,
		now,
		[]audit.FieldChange{
			{Field: "payment_attempt.reconciliation_attempt_count", Before: before.ReconciliationAttemptCount, After: after.ReconciliationAttemptCount},
			{Field: "payment_attempt.next_reconciliation_at", Before: optionalTime(before.NextReconciliationAt), After: optionalTime(after.NextReconciliationAt)},
		},
		metadata,
	)
}

func (uc *UseCase) newManualReviewAuditEvent(before, after model.PaymentAttempt, reason string, now time.Time) audit.Event {
	return newPaymentAttemptReconciliationAuditEvent(
		uc.idGenerator.Generate(),
		auditActionPaymentAttemptRequireManualReview,
		reason,
		audit.FailedOutcome,
		after,
		now,
		[]audit.FieldChange{{Field: "payment_attempt.state", Before: before.State, After: after.State}},
		map[string]any{"manual_review_required_at": optionalTime(after.ManualReviewRequiredAt)},
	)
}

func newPaymentAttemptReconciliationAuditEvent(
	eventID, action, reason string,
	outcome audit.Outcome,
	attempt model.PaymentAttempt,
	now time.Time,
	changes []audit.FieldChange,
	extra map[string]any,
) audit.Event {
	service := auditServicePaymentAttemptReconciliationTask
	metadata := map[string]any{
		"payment_attempt_id":           attempt.ID.String(),
		"contract_id":                  attempt.ContractID.String(),
		"installment_id":               attempt.InstallmentID.String(),
		"payment_purpose":              attempt.Purpose,
		"payment_channel":              attempt.Channel,
		"amount_minor":                 attempt.AmountMinor,
		"currency":                     attempt.Currency,
		"payment_attempt_state":        attempt.State,
		"reconciliation_attempt_count": attempt.ReconciliationAttemptCount,
		"reason_codes":                 []string{reason},
	}
	if attempt.ReconciliationReason != nil {
		metadata["reconciliation_reason"] = *attempt.ReconciliationReason
	}

	if attempt.FinancialPaymentIntentID != nil {
		metadata["financial_payment_intent_id"] = *attempt.FinancialPaymentIntentID
	}

	if attempt.FinancialPaymentAttemptID != nil {
		metadata["financial_payment_attempt_id"] = *attempt.FinancialPaymentAttemptID
	}

	if attempt.FinancialPaymentSessionID != nil {
		metadata["financial_payment_session_id"] = *attempt.FinancialPaymentSessionID
	}

	for key, value := range extra {
		if value != nil {
			metadata[key] = value
		}
	}

	reasonCode := reason
	return audit.NewEvent(
		eventID,
		"",
		nil,
		"",
		now,
		audit.EventData{
			Actor:      audit.Actor{Type: audit.SystemActorType, Service: &service},
			Action:     action,
			Resource:   audit.Resource{Type: "payment_attempt", ID: attempt.ID.String()},
			Outcome:    outcome,
			ReasonCode: &reasonCode,
			Changes:    changes,
			Metadata:   metadata,
		},
	)
}

func optionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC()
}
