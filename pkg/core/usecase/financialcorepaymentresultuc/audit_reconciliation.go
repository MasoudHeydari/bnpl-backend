package financialcorepaymentresultuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type reconciliationAuditFacts struct {
	InboxID                     model.InboxID
	Reason                      model.PaymentReconciliationReason
	Attempt                     *model.PaymentAttempt
	PreviousPaymentAttemptState *model.PaymentAttemptState
	CurrentPaymentAttemptState  *model.PaymentAttemptState
	ContractID                  model.ContractID
	InstallmentID               model.InstallmentID
	AttemptStateChanged         bool
}

func buildReconciliationAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	facts reconciliationAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "financialcorepaymentresultuc.buildReconciliationAuditEvent"

	metadata := basePaymentResultAuditMetadata(input, facts.InboxID, ProcessResultReconciliationRequired)
	metadata["reconciliation_reason"] = facts.Reason
	reasonCodes := normalizedReasonCodes(string(facts.Reason))
	putReasonCodes(metadata, reasonCodes)
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

	var changes []audit.FieldChange
	if facts.AttemptStateChanged &&
		facts.PreviousPaymentAttemptState != nil &&
		facts.CurrentPaymentAttemptState != nil {
		changes = append(changes, audit.FieldChange{
			Field:  "payment_attempt.state",
			Before: *facts.PreviousPaymentAttemptState,
			After:  *facts.CurrentPaymentAttemptState,
		})
	} else {
		changes = append(changes, audit.FieldChange{
			Field:  "payment_result.processing_state",
			Before: "received",
			After:  ProcessResultReconciliationRequired,
		})
	}

	causation := causationEventID(input)
	event := audit.NewEvent(
		eventID,
		correlationID,
		causation,
		traceID,
		now,
		audit.EventData{
			Actor:      auditActor(input.SourceService),
			Action:     auditActionPaymentResultRequireReconciliation,
			Resource:   resource,
			Outcome:    audit.FailedOutcome,
			ReasonCode: primaryReasonCode(reasonCodes),
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate reconciliation audit event: %w", op, err)
	}

	return event, nil
}

func (uc *UseCase) insertReconciliationAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	facts reconciliationAuditFacts,
	now time.Time,
) error {
	correlationID, traceID := resolveAuditEnvelopeIDs(ctx, input)
	event, err := buildReconciliationAuditEvent(
		uc.newAuditEventID(), correlationID, traceID, input, facts, now,
	)
	if err != nil {
		return err
	}

	return uc.insertAuditOutbox(ctx, outboxQ, event, now)
}

func buildPayloadConflictAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	inboxID model.InboxID,
	now time.Time,
) (audit.Event, error) {
	const op = "financialcorepaymentresultuc.buildPayloadConflictAuditEvent"

	metadata := basePaymentResultAuditMetadata(input, inboxID, ProcessResultPayloadConflictRecorded)
	metadata["reconciliation_reason"] = auditReasonPayloadIdentityConflict
	reasonCodes := normalizedReasonCodes(auditReasonPayloadIdentityConflict)
	putReasonCodes(metadata, reasonCodes)
	putOptionalUUID(metadata, "contract_id", input.ContractID)
	putOptionalUUID(metadata, "installment_id", input.InstallmentID)

	causation := causationEventID(input)
	event := audit.NewEvent(
		eventID,
		correlationID,
		causation,
		traceID,
		now,
		audit.EventData{
			Actor:      auditActor(input.SourceService),
			Action:     auditActionPaymentResultRequireReconciliation,
			Resource:   audit.Resource{Type: inboxResourceType, ID: inboxID.String()},
			Outcome:    audit.FailedOutcome,
			ReasonCode: primaryReasonCode(reasonCodes),
			Changes: []audit.FieldChange{
				{
					Field:  "payment_result.processing_state",
					Before: "received",
					After:  ProcessResultPayloadConflictRecorded,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate payload conflict audit event: %w", op, err)
	}

	return event, nil
}

func (uc *UseCase) insertPayloadConflictAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	inboxID model.InboxID,
	now time.Time,
) error {
	correlationID, traceID := resolveAuditEnvelopeIDs(ctx, input)
	event, err := buildPayloadConflictAuditEvent(
		uc.newAuditEventID(), correlationID, traceID, input, inboxID, now,
	)
	if err != nil {
		return err
	}

	return uc.insertAuditOutbox(ctx, outboxQ, event, now)
}
