package checkoutsessionuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

// LifecycleOutbox enqueues merchant-facing checkout lifecycle events in the
// caller's database transaction.
//
// Every event is written to the Kafka merchant-checkout-events destination and,
// when the merchant has an enabled webhook destination, also to a logical
// webhook://merchant/<merchant_id> outbox row. The HTTP dispatcher loads the
// current URL/secret at delivery time so disable/rotate/update applies to
// pending retries.
type LifecycleOutbox struct {
	checkoutSession repo.CheckoutSession
	merchantProfile repo.MerchantProfile
	outbox          repo.Outbox
	idGenerator     identifier.Generator
	destination     string
}

// NewLifecycleOutbox constructs a lifecycle event enqueuer.
func NewLifecycleOutbox(
	checkoutSession repo.CheckoutSession,
	merchantProfile repo.MerchantProfile,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	destination string,
) (*LifecycleOutbox, error) {
	const op = "checkoutsessionuc.NewLifecycleOutbox"

	if checkoutSession == nil {
		return nil, fmt.Errorf("%s: checkout session repository is required", op)
	}

	if merchantProfile == nil {
		return nil, fmt.Errorf("%s: merchant profile repository is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox repository is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if destination == "" {
		return nil, fmt.Errorf("%s: merchant checkout events destination is required", op)
	}

	return &LifecycleOutbox{
		checkoutSession: checkoutSession,
		merchantProfile: merchantProfile,
		outbox:          outbox,
		idGenerator:     idGenerator,
		destination:     destination,
	}, nil
}

// Enqueue inserts the lifecycle event for session's current state into the
// outbox inside tx.
//
// session must be the row as persisted by the transition the event describes.
// event_id is derived from the session public token + event type + status so
// retries of the same logical transition reuse one id. The internal session id
// never appears in the payload, event id, or message key.
func (l *LifecycleOutbox) Enqueue(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
	eventType string,
	now time.Time,
) error {
	const op = "checkoutsessionuc.LifecycleOutbox.Enqueue"

	merchantProfileQ := l.merchantProfile.Tx(tx)
	sessionQ := l.checkoutSession.Tx(tx)
	outboxQ := l.outbox.Tx(tx)

	profile, err := merchantProfileQ.GetByID(ctx, session.MerchantProfileID)
	if err != nil {
		return fmt.Errorf("%s: load merchant profile: %w", op, err)
	}

	data := checkoutlifecycle.EventData{
		CheckoutSessionID: session.PublicToken.String(),
		MerchantOrderID:   session.MerchantOrderID,
		MerchantID:        profile.MerchantID.String(),
		Status:            string(session.Status),
		AmountMinor:       session.RequestedAmountMinor,
		Currency:          string(session.Currency),
		MerchantState:     session.MerchantState,
	}

	if eventType == checkoutlifecycle.EventTypeCompleted {
		if session.CompletedAt != nil {
			completedAt := session.CompletedAt.UTC()
			data.CompletedAt = &completedAt
		} else {
			completedAt := now.UTC()
			data.CompletedAt = &completedAt
		}
	}

	if session.CancellationReason != nil {
		data.Reason = string(*session.CancellationReason)
	}

	// Stable event id for one logical transition (session + type + status).
	// Merchants see the event id, so it is derived from the public token and
	// never from the internal session id.
	eventID := session.PublicToken.String() + ":" + eventType + ":" + string(session.Status)

	event, err := checkoutlifecycle.NewEvent(eventID, eventType, now, data)
	if err != nil {
		return cerr.InternalError(fmt.Errorf("%s: build lifecycle event: %w", op, err))
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return cerr.InternalError(fmt.Errorf("%s: marshal lifecycle event: %w", op, err))
	}

	aggregateType := checkoutlifecycle.AggregateType

	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   l.destination,
		MessageKey:    session.PublicToken.String(),
		EventType:     eventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert lifecycle outbox message: %w", op, err)
	}

	// Optional HTTP webhook delivery via logical destination. Lookup is only to
	// decide whether to enqueue; URL/secret are resolved at dispatch time.
	_, webhookErr := sessionQ.GetWebhookDestination(ctx, profile.MerchantID)
	switch {
	case webhookErr == nil:
		webhookDestination := webhook.MerchantDestination(profile.MerchantID)

		_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
			Destination:   webhookDestination,
			MessageKey:    session.PublicToken.String(),
			EventType:     eventType,
			AggregateType: &aggregateType,
			Payload:       payload,
			CreatedAt:     now,
		})
		if err != nil {
			return fmt.Errorf("%s: insert webhook outbox message: %w", op, err)
		}

	case !cerr.IsCode(webhookErr, cerr.CodeResourceNotFound):
		return fmt.Errorf("%s: resolve webhook destination: %w", op, webhookErr)
	}

	return nil
}
