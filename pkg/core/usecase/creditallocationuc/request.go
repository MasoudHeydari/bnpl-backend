package creditallocationuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// PendingRequest is an administrative reference with no raw identity snapshot.
type PendingRequest struct {
	Request       model.CreditRequestSummary `json:"request"`
	CustomerID    model.CustomerID           `json:"customer_id"`
	CreditOwnerID model.CreditOwnerID        `json:"credit_owner_id"`
	CreditKind    model.CreditKind           `json:"credit_kind"`
}

func (uc *UseCase) ListPendingRequests(ctx context.Context, kind model.CreditKind, before time.Time, limit int) ([]PendingRequest, error) {
	if !kind.IsValid() {
		return nil, invalidInput("credit_kind")
	}
	if limit < 1 || limit > 500 {
		return nil, invalidInput("limit")
	}
	if before.IsZero() {
		before = uc.clock.Now().UTC()
	}
	out := make([]PendingRequest, 0)
	err := uc.tx(ctx, false, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		rows, err := q.ListPendingRequests(ctx, kind, before, limit)
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, PendingRequest{Request: r.Summary(), CustomerID: r.CustomerID, CreditOwnerID: r.CreditOwnerID, CreditKind: r.CreditKind})
		}
		return nil
	})
	return out, err
}
