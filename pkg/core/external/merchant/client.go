package merchant

import (
	"context"
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ErrBlocked indicates a conclusive contract or access error requiring correction.
var ErrBlocked = errors.New("merchant integration requires correction")

// ErrReferenceNotFound identifies a participation request absent at the source.
var ErrReferenceNotFound = errors.New("merchant participation reference not found")

// Reference is the minimum authoritative participation reference required by BNPL.
type Reference struct {
	MerchantID            model.ExternalMerchantID
	RequestID             model.MerchantParticipationRequestID
	BusinessLine          string
	Environment           string
	TradeName             string
	BaseApplicationStatus string
	SelfDeclaration       *model.MerchantSelfDeclaration
}

// Progress is the immutable READY report persisted by BNPL before delivery.
type Progress struct {
	RequestID        model.MerchantParticipationRequestID
	EventID          string
	OwnerVersion     int64
	AccountReference string
}

// Snapshot is Merchant's current projection, not necessarily the submitted version.
type Snapshot struct {
	RequestID        model.MerchantParticipationRequestID
	BusinessLine     string
	Environment      string
	Status           string
	OwnerVersion     int64
	AccountReference string
}

// Client reads participation references and reports complete READY snapshots.
// Transport failures are retryable unless ErrBlocked is returned.
type Client interface {
	GetParticipation(context.Context, model.MerchantParticipationRequestID) (Reference, error)
	ReportReady(context.Context, Progress) (Snapshot, error)
}
