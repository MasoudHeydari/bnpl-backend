// Package collectionpayment is the domain-facing port to Financial Core v2.
// It exposes no transport, provider credentials, FC ledger accounts or HTTP.
package collectionpayment

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/collectionpayment"
)

// CollectionStatus is owned by FC.
type CollectionStatus string
const (
	Prepared CollectionStatus = "prepared"
	Starting CollectionStatus = "starting"
	RequiresCustomerAction CollectionStatus = "requires_customer_action"
	ReconciliationRequired CollectionStatus = "reconciliation_required"
	AccountingPending CollectionStatus = "accounting_pending"
	Succeeded CollectionStatus = "succeeded"
	Failed CollectionStatus = "failed"
	Expired CollectionStatus = "expired"
	BankReversed CollectionStatus = "bank_reversed"
)

// PostingOutcome is distinct from provider success: only Posted may authorize
// final BNPL installment settlement and credit exposure release.
type PostingOutcome string
const (
	NotPosted PostingOutcome = "not_posted"
	PostingPending PostingOutcome = "pending"
	Posted PostingOutcome = "posted"
	ReversalPending PostingOutcome = "reversal_pending"
	Reversed PostingOutcome = "reversed"
)

// PostedAllocation is an FC-owned immutable posting reference.
type PostedAllocation struct {
	ItemID uuid.UUID
	ComponentID uuid.UUID
	AllocationID uuid.UUID
	AmountMinor uint64
	PostingReference string
}

// Result represents a durable snapshot of FC collection state.
type Result struct {
	CollectionPaymentID string
	SourceID uuid.UUID
	SnapshotRevision int64
	Status CollectionStatus
	PostingOutcome PostingOutcome
	PaymentIntentID string
	PaymentSessionID string
	PaymentLink string
	ProviderPaidAt *time.Time
	PostedAt *time.Time
	PostedAllocations []PostedAllocation
	ReasonCode string
}

// IsAuthoritativelySettled requires both successful collection and completed
// accounting. Provider success or a browser redirect alone never suffices.
func (r Result) IsAuthoritativelySettled() bool {
	return r.Status==Succeeded && r.PostingOutcome==Posted &&
		r.CollectionPaymentID!="" && r.PostedAt!=nil && !r.PostedAt.IsZero()
}

// Client is the agreed command/query boundary. GetBySource permits recovery
// when Prepare succeeded remotely but BNPL lost the response. Replaying Start
// must never create a second provider charge.
type Client interface {
	Prepare(ctx context.Context, plan collectionpayment.Plan, idempotencyKey string) (Result,error)
	Start(ctx context.Context, collectionPaymentID, idempotencyKey, providerCode string, revision int64) (Result,error)
	GetByID(ctx context.Context, collectionPaymentID string) (Result,error)
	GetBySource(ctx context.Context, sourceID uuid.UUID) (Result,error)
}
