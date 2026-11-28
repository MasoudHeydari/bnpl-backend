package merchantparticipation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantprovisioninguc"
)

const eventType = "merchant_product_participation_requested"

type requestedEvent struct {
	EventID           uuid.UUID                            `json:"event_id"`
	EventType         string                               `json:"event_type"`
	SchemaVersion     int                                  `json:"schema_version"`
	CorrelationID     uuid.UUID                            `json:"correlation_id"`
	OccurredAt        time.Time                            `json:"occurred_at"`
	ActorSubjectID    uuid.UUID                            `json:"actor_subject_id"`
	ActorMembershipID *uuid.UUID                           `json:"actor_membership_id"`
	ActorContextID    *uuid.UUID                           `json:"actor_context_id"`
	MerchantID        model.ExternalMerchantID             `json:"merchant_id"`
	ApplicationID     model.MerchantApplicationID          `json:"application_id"`
	RequestID         model.MerchantParticipationRequestID `json:"request_id"`
	BusinessLine      string                               `json:"business_line_code"`
	Environment       string                               `json:"environment"`
}

// Handler validates the transport envelope and delegates business processing.
type Handler struct {
	uc     *merchantprovisioninguc.UseCase
	hasher fingerprint.Hasher
}

func New(uc *merchantprovisioninguc.UseCase, hasher fingerprint.Hasher) (*Handler, error) {
	const op = "merchantparticipation.New"
	if uc == nil || hasher == nil {
		return nil, fmt.Errorf("%s: use case and fingerprint hasher required", op)
	}

	return &Handler{uc: uc, hasher: hasher}, nil
}

func (h *Handler) Handle(ctx context.Context, msg messaging.Message) error {
	input, err := decode(msg, h.hasher)
	if err != nil {
		return messaging.NonRetryable(err)
	}

	err = h.uc.Process(ctx, input)
	if errors.Is(err, merchantprovisioninguc.ErrConflict) || errors.Is(err, merchantprovisioninguc.ErrInvalidReference) {
		return messaging.NonRetryable(err)
	}

	return err
}

func decode(msg messaging.Message, hasher fingerprint.Hasher) (merchantprovisioninguc.Input, error) {
	const op = "merchantparticipation.decode"
	var input merchantprovisioninguc.Input
	var event requestedEvent
	decoder := json.NewDecoder(bytes.NewReader(msg.Value))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&event)
	if err != nil || decoder.Decode(new(any)) != io.EOF {
		return input, fmt.Errorf("%s: invalid participation envelope", op)
	}

	if event.EventID == uuid.Nil || event.CorrelationID == uuid.Nil || event.ActorSubjectID == uuid.Nil || event.OccurredAt.IsZero() || event.MerchantID.IsZero() || event.ApplicationID.IsZero() || event.RequestID.IsZero() || event.EventType != eventType || event.SchemaVersion != 1 || event.BusinessLine != "bnpl" || event.Environment != "SANDBOX" {
		return input, fmt.Errorf("%s: invalid or unsupported participation identity, version or environment", op)
	}

	if string(msg.Key) != event.MerchantID.String() {
		return input, fmt.Errorf("%s: merchant key mismatch", op)
	}

	expected := map[string]string{
		"x-event-id": event.EventID.String(), "x-event-type": eventType,
		"x-aggregate-type": "merchant_enrollment", "x-aggregate-id": event.MerchantID.String(), "x-schema-version": "1",
	}
	for key, value := range expected {
		if string(msg.Headers[key]) != value {
			return input, fmt.Errorf("%s: missing or mismatched %s", op, key)
		}
	}

	correlation := event.CorrelationID.String()
	if header, ok := msg.Headers["x-correlation-id"]; ok && string(header) != correlation {
		return input, fmt.Errorf("%s: correlation header mismatch", op)
	}

	// Canonicalize the known wire DTO so insignificant JSON formatting differences
	// do not alter replay identity. The command identity is checked independently.
	canonical, err := json.Marshal(event)
	if err != nil {
		return input, fmt.Errorf("%s: normalize envelope: %w", op, err)
	}

	hash, err := hasher.Hash(string(canonical))
	if err != nil {
		return input, fmt.Errorf("%s: fingerprint envelope: %w", op, err)
	}

	return merchantprovisioninguc.Input{
		Event:      model.InboxEvent{EventID: event.EventID.String(), EventType: eventType, EventVersion: "1", SourceService: "merchant_service", CorrelationID: &correlation, Payload: canonical, PayloadFingerprint: hash.String(), ConsumerName: "merchant_participation"},
		MerchantID: event.MerchantID, ApplicationID: event.ApplicationID, RequestID: event.RequestID, Environment: event.Environment,
	}, nil
}
