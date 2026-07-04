package financialcorepaymentreconciliationuc

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
	auditActionResolveSubmitted = "payment_attempt.resolve_reconciliation_as_submitted"
	auditActionResolveSucceeded = "payment_attempt.resolve_reconciliation_as_succeeded"
	auditActionResolveFailed    = "payment_attempt.resolve_reconciliation_as_failed"
	auditActionResolveCanceled  = "payment_attempt.resolve_reconciliation_as_canceled"
	auditActionKeepRequired     = "payment_attempt.keep_reconciliation_required"

	auditReasonResolvedSubmitted   = "financial_core_reconciliation_resolved_submitted"
	auditReasonResolvedSucceeded   = "financial_core_reconciliation_resolved_succeeded"
	auditReasonResolvedFailed      = "financial_core_reconciliation_resolved_failed"
	auditReasonResolvedCanceled    = "financial_core_reconciliation_resolved_canceled"
	auditReasonApplicationConflict = "reconciliation_application_conflict"
)

func (uc *UseCase) insertResolutionAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	action string,
	reason string,
	includeStateChange bool,
	now time.Time,
) error {
	const op = "financialcorepaymentreconciliationuc.UseCase.insertResolutionAudit"

	if uc.auditDestination == "" {
		return fmt.Errorf("%s: audit destination is not configured", op)
	}

	correlationID := strings.TrimSpace(stringValue(input.CorrelationID))
	if correlationID == "" {
		correlationID = strings.TrimSpace(trace.CorrelationID(ctx))
	}

	if correlationID == "" {
		correlationID = input.EventID
	}

	traceID := strings.TrimSpace(trace.ID(ctx))
	if traceID == "" {
		traceID = correlationID
	}

	causationID := input.EventID
	reasonCode := reason
	metadata := map[string]any{
		"payment_attempt_id":             attempt.ID.String(),
		"previous_payment_attempt_state": previousState,
		"current_payment_attempt_state":  attempt.State,
		"reconciliation_generation":      input.Data.ReconciliationGeneration,
		"financial_core_result":          input.Data.Status,
		"financial_core_event_id":        input.EventID,
		"financial_core_event_type":      input.EventType,
		"financial_core_event_version":   input.EventVersion,
		"financial_core_source_service":  input.SourceService,
		"process_reason":                 reason,
		"reason_codes":                   []string{reason},
	}

	if !attempt.ContractID.IsZero() {
		metadata["contract_id"] = attempt.ContractID.String()
	}

	if !attempt.InstallmentID.IsZero() {
		metadata["installment_id"] = attempt.InstallmentID.String()
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

	if input.Data.FailureCode != nil && strings.TrimSpace(*input.Data.FailureCode) != "" {
		metadata["failure_code"] = strings.TrimSpace(*input.Data.FailureCode)
	}

	if action == auditActionResolveSucceeded {
		metadata["financial_mutations_audited_by"] = "financialcorepaymentresultuc"
	}

	changes := []audit.FieldChange(nil)
	if includeStateChange && previousState != attempt.State {
		changes = []audit.FieldChange{{
			Field:  "payment_attempt.state",
			Before: previousState,
			After:  attempt.State,
		}}
	}

	service := input.SourceService
	event := audit.NewEvent(
		uc.idGenerator.Generate(),
		correlationID,
		&causationID,
		traceID,
		now,
		audit.EventData{
			Actor:      audit.Actor{Type: audit.ServiceActorType, Service: &service},
			Action:     action,
			Resource:   audit.Resource{Type: "payment_attempt", ID: attempt.ID.String()},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate payment reconciliation audit event: %w", op, err)
	}

	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal payment reconciliation audit event: %w", op, err)
	}

	aggregateType := "payment_attempt"
	if _, err = outboxQ.Create(
		ctx,
		repo.CreateOutboxMessage{
			Destination:   uc.auditDestination,
			MessageKey:    attempt.ID.String(),
			EventType:     audit.EventType,
			AggregateType: &aggregateType,
			Payload:       raw,
			CreatedAt:     now,
		},
	); err != nil {
		return fmt.Errorf("%s: insert payment reconciliation audit outbox message: %w", op, err)
	}

	return nil
}
