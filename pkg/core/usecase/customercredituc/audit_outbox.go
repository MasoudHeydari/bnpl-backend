package customercredituc

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
	assignInitialLimitAuditAction = "credit_account.assign_initial_limit"
	customerProfileResourceType   = "customer_profile"
	creditAccountResourceType     = "credit_account"
)

// insertAssignInitialLimitAuditOutbox records a newly concluded initial-limit
// assessment in the same transaction as the Decision/CreditAccount mutation.
func (uc *UseCase) insertAssignInitialLimitAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	profile model.CustomerProfile,
	output AssignInitialLimitOutput,
	assessment InitialLimitAssessmentResult,
	now time.Time,
) error {
	const op = "customercredituc.UseCase.insertAssignInitialLimitAuditOutbox"
	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: assign initial limit audit envelope ids: %w", op, err)
	}

	event, err := buildAssignInitialLimitAuditEvent(uc.idGenerator.Generate(), correlationID, traceID, profile, output, assessment, now)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal assign initial limit audit event: %w", op, err)
	}

	aggregateType := event.Data.Resource.Type
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination: uc.auditDestination, MessageKey: event.Data.Resource.ID,
		EventType: audit.EventType, AggregateType: &aggregateType, Payload: payload, CreatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert assign initial limit audit outbox message: %w", op, err)
	}

	return nil
}

func buildAssignInitialLimitAuditEvent(
	eventID, correlationID, traceID string,
	profile model.CustomerProfile,
	output AssignInitialLimitOutput,
	assessment InitialLimitAssessmentResult,
	now time.Time,
) (audit.Event, error) {
	const op = "customercredituc.buildAssignInitialLimitAuditEvent"
	serviceName := audit.ProducerName
	reasonCodes := append([]string(nil), output.Decision.ReasonCodes...)
	metadata := map[string]any{
		"customer_id": profile.ID.String(), "identity_subject_id": profile.IdentitySubjectID.String(),
		"decision_id": output.Decision.ID.String(), "policy_id": assessment.PolicyID,
		"policy_version": assessment.PolicyVersion, "decision_outcome": output.Decision.Outcome,
		"reason_codes": reasonCodes, "currency": assessment.Currency,
	}

	resource := audit.Resource{Type: customerProfileResourceType, ID: profile.ID.String()}
	outcome := audit.DeniedOutcome
	changes := []audit.FieldChange{{Field: "decision.outcome", Before: nil, After: output.Decision.Outcome}}
	if output.CreditLimit != nil {
		account := output.CreditLimit
		resource = audit.Resource{Type: creditAccountResourceType, ID: account.ID.String()}
		outcome = audit.SucceededOutcome
		metadata["credit_account_id"] = account.ID.String()
		metadata["limit_amount_minor"] = account.LimitAmountMinor
		metadata["exposure_amount_minor"] = account.ExposureAmountMinor
		metadata["status"] = account.Status
		metadata["effective_from"] = account.EffectiveFrom
		metadata["expires_at"] = account.ExpiresAt
		changes = append(changes,
			audit.FieldChange{Field: "credit_account.status", Before: nil, After: account.Status},
			audit.FieldChange{Field: "credit_account.limit_amount_minor", Before: nil, After: account.LimitAmountMinor},
		)
	}

	event := audit.NewEvent(eventID, correlationID, nil, traceID, now, audit.EventData{
		Actor:  audit.Actor{Type: audit.SystemActorType, Service: &serviceName},
		Action: assignInitialLimitAuditAction, Resource: resource, Outcome: outcome,
		ReasonCode: firstReasonCode(reasonCodes), Changes: changes, Metadata: metadata,
	})
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate assign initial limit audit event: %w", op, err)
	}

	return event, nil
}

func firstReasonCode(reasonCodes []string) *string {
	if len(reasonCodes) == 0 {
		return nil
	}

	reasonCode := reasonCodes[0]
	return &reasonCode
}
