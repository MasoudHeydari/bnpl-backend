// Package collectionpayment is the domain-facing port to Financial Core v2.
// It exposes no transport, provider credentials, FC ledger accounts or HTTP.
package collectionpayment

import (
	"context"
	"fmt"
	"strings"
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


// VerifyPostedAllocations checks the exact immutable BNPL allocation snapshot
// against FC's posted accounting response. It must run before a local batch
// transitions to settled or releases exposure. Unexpected/missing postings fail
// closed and require reconciliation rather than marking a partial settlement.
func VerifyPostedAllocations(plan collectionpayment.Plan, result Result) error {
	const op = "collectionpayment.VerifyPostedAllocations"
	if err := plan.Validate(); err != nil {
		return fmt.Errorf("%s: invalid plan: %w", op, err)
	}
	if !result.IsAuthoritativelySettled() || result.SourceID != plan.ID ||
		result.SnapshotRevision < 1 {
		return fmt.Errorf("%s: FC result is not an authoritative match", op)
	}

	type posting struct {
		item uuid.UUID
		component uuid.UUID
		amount uint64
	}
	expected := make(map[uuid.UUID]posting)
	for _, item := range plan.Items {
		for _, component := range item.Components {
			for _, allocation := range component.Allocations {
				expected[allocation.ID] = posting{
					item: item.ID, component: component.ID, amount: allocation.AmountMinor,
				}
			}
		}
	}
	if len(expected) != len(result.PostedAllocations) {
		return fmt.Errorf("%s: posted allocation count mismatch", op)
	}
	seen := make(map[uuid.UUID]struct{}, len(result.PostedAllocations))
	for _, actual := range result.PostedAllocations {
		exp, ok := expected[actual.AllocationID]
		if !ok || exp.item != actual.ItemID ||
			exp.component != actual.ComponentID || exp.amount != actual.AmountMinor ||
			strings.TrimSpace(actual.PostingReference) == "" {
			return fmt.Errorf("%s: posted allocation does not match snapshot", op)
		}
		if _, duplicate := seen[actual.AllocationID]; duplicate {
			return fmt.Errorf("%s: duplicate posted allocation", op)
		}
		seen[actual.AllocationID] = struct{}{}
	}
	return nil
}
