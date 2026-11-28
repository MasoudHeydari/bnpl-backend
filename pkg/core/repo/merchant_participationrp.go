package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MerchantParticipationQueryer reads durable participation commands.
type MerchantParticipationQueryer interface {
	Get(context.Context, model.MerchantParticipationRequestID) (model.MerchantParticipation, error)
}

// MerchantParticipationTxQueryer writes commands and leases callback deliveries.
type MerchantParticipationTxQueryer interface {
	MerchantParticipationQueryer
	Insert(context.Context, model.MerchantParticipation, time.Time) error
	ClaimDue(context.Context, time.Time, time.Time) (model.MerchantParticipation, error)
	Finish(context.Context, MerchantProgressDeliveryResult) error
}

// MerchantProgressDeliveryResult is conditional on the claimed attempt generation.
type MerchantProgressDeliveryResult struct {
	RequestID           model.MerchantParticipationRequestID
	DeclarationRevision int64
	AttemptCount        int64
	Status              string
	Reason              string
	NextAttemptAt       time.Time
	Now                 time.Time
}

// MerchantParticipation persists participation and independently tracks callback delivery.
type MerchantParticipation interface {
	Conn(Conn) MerchantParticipationQueryer
	Tx(Tx) MerchantParticipationTxQueryer
}
