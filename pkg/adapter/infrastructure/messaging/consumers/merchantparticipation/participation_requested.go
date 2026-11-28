package merchantparticipation

import (
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

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
