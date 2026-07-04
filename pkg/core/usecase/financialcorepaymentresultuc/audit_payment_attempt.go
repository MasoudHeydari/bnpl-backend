package financialcorepaymentresultuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type paymentAttemptTerminalAuditFacts struct {
	InboxID       model.InboxID
	Attempt       model.PaymentAttempt
	PreviousState model.PaymentAttemptState
	CurrentState  model.PaymentAttemptState
	ProcessResult ProcessResult
	ReasonCode    string
	ContractID    model.ContractID
	InstallmentID model.InstallmentID
}

func buildPaymentAttemptFailedAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	facts paymentAttemptTerminalAuditFacts,
	now time.Time,
) (audit.Event, error) {
	return buildPaymentAttemptTerminalAuditEvent(
		eventID,
		correlationID,
		traceID,
		input,
		facts,
		auditActionPaymentAttemptMarkFailed,
		audit.FailedOutcome,
		now,
	)
}

func buildPaymentAttemptCanceledAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	facts paymentAttemptTerminalAuditFacts,
	now time.Time,
) (audit.Event, error) {
	// SucceededOutcome means the cancellation mutation was successfully recorded;
	// it does not mean the underlying payment succeeded. Payment status remains in Metadata.
	return buildPaymentAttemptTerminalAuditEvent(
		eventID,
		correlationID,
		traceID,
		input,
		facts,
		auditActionPaymentAttemptMarkCanceled,
		audit.SucceededOutcome,
		now,
	)
}

func buildPaymentAttemptTerminalAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	facts paymentAttemptTerminalAuditFacts,
	action string,
	outcome audit.Outcome,
	now time.Time,
) (audit.Event, error) {
	const op = "financialcorepaymentresultuc.buildPaymentAttemptTerminalAuditEvent"

	metadata := basePaymentResultAuditMetadata(input, facts.InboxID, facts.ProcessResult)
	metadata["payment_attempt_id"] = facts.Attempt.ID.String()
	putOptionalUUID(metadata, "contract_id", facts.ContractID)
	putOptionalUUID(metadata, "installment_id", facts.InstallmentID)
	metadata["previous_payment_attempt_state"] = facts.PreviousState
	metadata["current_payment_attempt_state"] = facts.CurrentState

	reason := facts.ReasonCode
	if reason == "" && action == auditActionPaymentAttemptMarkCanceled {
		reason = auditReasonPaymentAttemptCanceled
	}

	reasonCodes := normalizedReasonCodes(reason)
	putReasonCodes(metadata, reasonCodes)

	causation := causationEventID(input)
	event := audit.NewEvent(
		eventID,
		correlationID,
		causation,
		traceID,
		now,
		audit.EventData{
			Actor:      auditActor(input.SourceService),
			Action:     action,
			Resource:   audit.Resource{Type: paymentAttemptResourceType, ID: facts.Attempt.ID.String()},
			Outcome:    outcome,
			ReasonCode: primaryReasonCode(reasonCodes),
			Changes: []audit.FieldChange{
				{
					Field:  "payment_attempt.state",
					Before: facts.PreviousState,
					After:  facts.CurrentState,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate payment attempt terminal audit event: %w", op, err)
	}

	return event, nil
}

func (uc *UseCase) insertPaymentAttemptFailedAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	facts paymentAttemptTerminalAuditFacts,
	now time.Time,
) error {
	correlationID, traceID := resolveAuditEnvelopeIDs(ctx, input)
	event, err := buildPaymentAttemptFailedAuditEvent(
		uc.newAuditEventID(),
		correlationID,
		traceID,
		input,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	return uc.insertAuditOutbox(ctx, outboxQ, event, now)
}

func (uc *UseCase) insertPaymentAttemptCanceledAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	facts paymentAttemptTerminalAuditFacts,
	now time.Time,
) error {
	correlationID, traceID := resolveAuditEnvelopeIDs(ctx, input)
	event, err := buildPaymentAttemptCanceledAuditEvent(
		uc.newAuditEventID(),
		correlationID,
		traceID,
		input,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	return uc.insertAuditOutbox(ctx, outboxQ, event, now)
}

type unmatchedTerminalAuditFacts struct {
	InboxID       model.InboxID
	FailureCode   string
	Attempt       *model.PaymentAttempt
	ContractID    model.ContractID
	InstallmentID model.InstallmentID
}

func buildUnmatchedTerminalAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	facts unmatchedTerminalAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "financialcorepaymentresultuc.buildUnmatchedTerminalAuditEvent"

	metadata := basePaymentResultAuditMetadata(input, facts.InboxID, ProcessResultTerminalResultUnmatched)
	metadata["reconciliation_reason"] = facts.FailureCode
	putOptionalUUID(metadata, "contract_id", facts.ContractID)
	putOptionalUUID(metadata, "installment_id", facts.InstallmentID)

	resource := audit.Resource{Type: inboxResourceType, ID: facts.InboxID.String()}
	if facts.Attempt != nil {
		resource = audit.Resource{Type: paymentAttemptResourceType, ID: facts.Attempt.ID.String()}
		metadata["payment_attempt_id"] = facts.Attempt.ID.String()
	} else if !facts.InstallmentID.IsZero() {
		resource = audit.Resource{Type: installmentResourceType, ID: facts.InstallmentID.String()}
	} else if !facts.ContractID.IsZero() {
		resource = audit.Resource{Type: contractResourceType, ID: facts.ContractID.String()}
	}

	reasonCodes := normalizedReasonCodes(facts.FailureCode)
	putReasonCodes(metadata, reasonCodes)

	causation := causationEventID(input)
	event := audit.NewEvent(
		eventID,
		correlationID,
		causation,
		traceID,
		now,
		audit.EventData{
			Actor:      auditActor(input.SourceService),
			Action:     auditActionPaymentResultRecordUnmatchedTerminalResult,
			Resource:   resource,
			Outcome:    audit.FailedOutcome,
			ReasonCode: primaryReasonCode(reasonCodes),
			Changes: []audit.FieldChange{
				{
					Field:  "payment_result.processing_state",
					Before: "received",
					After:  ProcessResultTerminalResultUnmatched,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate unmatched terminal audit event: %w", op, err)
	}

	return event, nil
}

func (uc *UseCase) insertUnmatchedTerminalAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	facts unmatchedTerminalAuditFacts,
	now time.Time,
) error {
	correlationID, traceID := resolveAuditEnvelopeIDs(ctx, input)
	event, err := buildUnmatchedTerminalAuditEvent(
		uc.newAuditEventID(),
		correlationID,
		traceID,
		input,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	return uc.insertAuditOutbox(ctx, outboxQ, event, now)
}
