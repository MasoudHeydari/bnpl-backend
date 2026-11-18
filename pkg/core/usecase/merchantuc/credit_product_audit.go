package merchantuc

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
	// setCreditProductEnabledAuditAction is the audit action for eligibility changes.
	setCreditProductEnabledAuditAction = "merchant_credit_product.set_enabled"

	// merchantProfileResourceType is the audit resource type for merchant profiles.
	merchantProfileResourceType = "merchant_profile"
)

// insertSetEnabledAuditOutbox records an eligibility change in the same
// transaction that wrote it.
func (uc *CreditProductUseCase) insertSetEnabledAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	profile model.MerchantProfile,
	input SetCreditProductEnabledInput,
	previous bool,
	now time.Time,
) error {
	const op = "merchantuc.CreditProductUseCase.insertSetEnabledAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: envelope ids: %w", op, err)
	}

	event, err := buildSetEnabledAuditEvent(
		uc.idGenerator.Generate(),
		correlationID,
		traceID,
		profile,
		input,
		previous,
		now,
	)
	if err != nil {
		return fmt.Errorf("%s: build event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal event: %w", op, err)
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
		return fmt.Errorf("%s: insert outbox message: %w", op, err)
	}

	return nil
}

func buildSetEnabledAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	profile model.MerchantProfile,
	input SetCreditProductEnabledInput,
	previous bool,
	now time.Time,
) (audit.Event, error) {
	const op = "merchantuc.buildSetEnabledAuditEvent"

	actorID := input.ActorSubjectID.String()
	event := audit.NewEvent(
		eventID,
		correlationID,
		nil,
		traceID,
		now,
		audit.EventData{
			Actor:  audit.Actor{Type: audit.OperatorActorType, ID: &actorID},
			Action: setCreditProductEnabledAuditAction,
			Resource: audit.Resource{
				Type: merchantProfileResourceType,
				ID:   profile.ID.String(),
			},
			Outcome: audit.SucceededOutcome,
			Changes: []audit.FieldChange{{
				Field:  "credit_product." + string(input.CreditKind) + ".enabled",
				Before: previous,
				After:  input.Enabled,
			}},
			Metadata: map[string]any{
				"merchant_profile_id":  profile.ID.String(),
				"external_merchant_id": profile.ExternalMerchantID.String(),
				"credit_kind":          string(input.CreditKind),
				"enabled":              input.Enabled,
			},
		},
	)

	err := audit.ValidateEvent(event)
	if err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate event: %w", op, err)
	}

	return event, nil
}
