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

// ErrDeclarationRevisionConflict requires a fresh snapshot, not another report.
var ErrDeclarationRevisionConflict = errors.New("merchant declaration revision conflict")

// Reference is the authoritative participation reference required by BNPL.
type Reference struct {
	MerchantID            model.ExternalMerchantID
	ApplicationID         model.MerchantApplicationID
	RequestID             model.MerchantParticipationRequestID
	BusinessLine          string
	Environment           string
	TradeName             string
	BaseApplicationStatus string
	Status                string
	OwnerVersion          int64
	DeclarationRevision   int64
	SelfDeclaration       *model.MerchantSelfDeclaration
}

// Progress is an immutable report persisted by BNPL before delivery.
type Progress struct {
	RequestID           model.MerchantParticipationRequestID
	EventID             string
	OwnerVersion        int64
	DeclarationRevision int64
	Status              string
	AccountReference    string
	Issues              []model.MerchantParticipationIssue
}

// Snapshot is Merchant's current projection, not necessarily the submitted version.
type Snapshot struct {
	RequestID           model.MerchantParticipationRequestID
	BusinessLine        string
	Environment         string
	Status              string
	OwnerVersion        int64
	DeclarationRevision int64
	AccountReference    string
	Issues              []model.MerchantParticipationIssue
}

// OutletEligibility reports the source-owned eligibility, not panel authorization.
type OutletEligibility struct {
	Eligible    bool
	Environment string
}

// Client reads participation references and reports immutable progress snapshots.
// Transport failures are retryable unless ErrBlocked is returned.
type Client interface {
	GetParticipation(context.Context, model.MerchantParticipationRequestID) (Reference, error)
	ReportProgress(context.Context, Progress) (Snapshot, error)
	GetOutletSetupEligibility(context.Context, model.ExternalMerchantID, model.ExternalBranchID) (OutletEligibility, error)
}
