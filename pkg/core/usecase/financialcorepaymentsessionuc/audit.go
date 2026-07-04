package financialcorepaymentsessionuc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

const (
	paymentSessionSubmittedAuditAction = "payment_attempt.attach_financial_payment_session"
	paymentAttemptResourceType         = "payment_attempt"
	paymentSessionSubmittedReasonCode  = "financial_payment_session_attached"
)

func (uc *UseCase) insertPaymentSessionSubmittedAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	inbox model.Inbox,
	input ProcessInput,
	beforeState model.PaymentAttemptState,
	submitted model.PaymentAttempt,
	now time.Time,
) error {
	const op = "financialcorepaymentsessionuc.UseCase.insertPaymentSessionSubmittedAudit"

	correlationID := strings.TrimSpace(trace.CorrelationID(ctx))
	if input.CorrelationID != nil && strings.TrimSpace(*input.CorrelationID) != "" {
		correlationID = strings.TrimSpace(*input.CorrelationID)
	}

	if correlationID == "" {
		correlationID = input.EventID
	}

	traceID := strings.TrimSpace(trace.ID(ctx))
	if traceID == "" {
		traceID = correlationID
	}

	causationID := input.EventID
	service := input.SourceService
	reasonCode := paymentSessionSubmittedReasonCode
	metadata := map[string]any{
		"payment_attempt_id":                   submitted.ID.String(),
		"contract_id":                          submitted.ContractID.String(),
		"installment_id":                       submitted.InstallmentID.String(),
		"previous_payment_attempt_state":       beforeState,
		"current_payment_attempt_state":        submitted.State,
		"payment_purpose":                      submitted.Purpose,
		"payment_channel":                      submitted.Channel,
		"amount_minor":                         submitted.AmountMinor,
		"currency":                             submitted.Currency,
		"financial_payment_intent_id":          input.FinancialPaymentIntentID,
		"financial_payment_session_id":         input.FinancialPaymentSessionID,
		"financial_core_event_id":              input.EventID,
		"financial_core_event_type":            input.EventType,
		"financial_core_event_version":         input.EventVersion,
		"financial_core_source_service":        input.SourceService,
		"financial_core_event_occurred_at":     input.EventOccurredAt.UTC(),
		"financial_payment_session_expires_at": input.ExpiresAt.UTC(),
		"inbox_id":                             inbox.ID.String(),
		"process_result":                       ProcessResultSessionSubmitted,
		"reason_codes":                         []string{paymentSessionSubmittedReasonCode},
	}

	if input.FinancialPaymentAttemptID != nil {
		metadata["financial_payment_attempt_id"] = *input.FinancialPaymentAttemptID
	}

	if input.CausationID != nil && strings.TrimSpace(*input.CausationID) != "" {
		metadata["upstream_causation_id"] = strings.TrimSpace(*input.CausationID)
	}

	if submitted.SubmittedAt != nil {
		metadata["submitted_at"] = submitted.SubmittedAt.UTC()
	}

	event := audit.NewEvent(
		uc.idGenerator.Generate(),
		correlationID,
		&causationID,
		traceID,
		now,
		audit.EventData{
			Actor: audit.Actor{
				Type:    audit.ServiceActorType,
				Service: &service,
			},
			Action: paymentSessionSubmittedAuditAction,
			Resource: audit.Resource{
				Type: paymentAttemptResourceType,
				ID:   submitted.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{
					Field:  "payment_attempt.state",
					Before: beforeState,
					After:  submitted.State,
				},
				{
					Field:  "payment_attempt.financial_payment_session_id",
					Before: nil,
					After:  input.FinancialPaymentSessionID,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate financial payment session audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal financial payment session audit event: %w", op, err)
	}

	aggregateType := paymentAttemptResourceType
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    submitted.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert financial payment session audit outbox message: %w", op, err)
	}

	return nil
}

const (
	paymentSessionIdentityConflictAuditAction = "payment_attempt.require_reconciliation"
	paymentSessionIdentityConflictReasonCode  = "financial_payment_session_identity_conflict"
)

func (uc *UseCase) insertPaymentSessionIdentityConflictAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	inbox model.Inbox,
	input ProcessInput,
	beforeState model.PaymentAttemptState,
	attempt model.PaymentAttempt,
	now time.Time,
) error {
	const op = "financialcorepaymentsessionuc.UseCase.insertPaymentSessionIdentityConflictAudit"

	correlationID := strings.TrimSpace(trace.CorrelationID(ctx))
	if input.CorrelationID != nil && strings.TrimSpace(*input.CorrelationID) != "" {
		correlationID = strings.TrimSpace(*input.CorrelationID)
	}

	if correlationID == "" {
		correlationID = input.EventID
	}

	traceID := strings.TrimSpace(trace.ID(ctx))
	if traceID == "" {
		traceID = correlationID
	}

	causationID := input.EventID
	service := input.SourceService
	reasonCode := paymentSessionIdentityConflictReasonCode
	metadata := map[string]any{
		"payment_attempt_id":               attempt.ID.String(),
		"contract_id":                      attempt.ContractID.String(),
		"installment_id":                   attempt.InstallmentID.String(),
		"previous_payment_attempt_state":   beforeState,
		"current_payment_attempt_state":    attempt.State,
		"payment_purpose":                  attempt.Purpose,
		"payment_channel":                  attempt.Channel,
		"amount_minor":                     attempt.AmountMinor,
		"currency":                         attempt.Currency,
		"financial_payment_intent_id":      input.FinancialPaymentIntentID,
		"financial_payment_session_id":     input.FinancialPaymentSessionID,
		"financial_core_event_id":          input.EventID,
		"financial_core_event_type":        input.EventType,
		"financial_core_event_version":     input.EventVersion,
		"financial_core_source_service":    input.SourceService,
		"financial_core_event_occurred_at": input.EventOccurredAt.UTC(),
		"inbox_id":                         inbox.ID.String(),
		"process_result":                   ProcessResultSessionIdentityConflict,
		"reason_codes":                     []string{reasonCode},
	}

	if input.FinancialPaymentAttemptID != nil {
		metadata["financial_payment_attempt_id"] = *input.FinancialPaymentAttemptID
	}

	if input.CausationID != nil && strings.TrimSpace(*input.CausationID) != "" {
		metadata["upstream_causation_id"] = strings.TrimSpace(*input.CausationID)
	}

	if attempt.ReconciliationReason != nil {
		metadata["reconciliation_reason"] = *attempt.ReconciliationReason
	}

	if attempt.NextReconciliationAt != nil {
		metadata["next_reconciliation_at"] = attempt.NextReconciliationAt.UTC()
	}

	event := audit.NewEvent(
		uc.idGenerator.Generate(),
		correlationID,
		&causationID,
		traceID,
		now,
		audit.EventData{
			Actor: audit.Actor{
				Type:    audit.ServiceActorType,
				Service: &service,
			},
			Action: paymentSessionIdentityConflictAuditAction,
			Resource: audit.Resource{
				Type: paymentAttemptResourceType,
				ID:   attempt.ID.String(),
			},
			Outcome:    audit.FailedOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{
					Field:  "payment_attempt.state",
					Before: beforeState,
					After:  attempt.State,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate financial payment session identity conflict audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal financial payment session identity conflict audit event: %w", op, err)
	}

	aggregateType := paymentAttemptResourceType
	if _, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    attempt.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	}); err != nil {
		return fmt.Errorf("%s: insert financial payment session identity conflict audit outbox message: %w", op, err)
	}

	return nil
}
