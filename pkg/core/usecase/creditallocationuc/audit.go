package creditallocationuc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// auditChange records configuration changes transactionally. Never include
// raw national codes or mobile aliases in the audit event.
func (uc *UseCase) auditChange(ctx context.Context, tx repo.Tx, actor, action, resourceType, resourceID string, metadata map[string]any, now time.Time) error {
	correlation, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return err
	}
	event := audit.NewEvent(uc.idGenerator.Generate(), correlation, nil, traceID, now, audit.EventData{Actor: audit.Actor{Type: audit.OperatorActorType, ID: &actor}, Action: action, Resource: audit.Resource{Type: resourceType, ID: resourceID}, Outcome: audit.SucceededOutcome, Metadata: metadata})
	if err := audit.ValidateEvent(event); err != nil {
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	outboxQ := uc.outbox.Tx(tx)
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    resourceID,
		EventType:     audit.EventType,
		AggregateType: &resourceType,
		Payload:       payload,
		CreatedAt:     now,
	})
	return err
}
