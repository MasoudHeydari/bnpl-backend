package creditallocationuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase orchestrates administrative configuration changes. Configuration
// edits and queue authorization are committed in the same transaction.
type UseCase struct {
	pool             repo.Pool
	allocation       repo.CreditAllocation
	trusted          repo.TrustedMobile
	clock            clock.Clock
	installments     initialcreditlimit.Policy
	monthly          initialcreditlimit.Policy
	outbox           repo.Outbox
	idGenerator      identifier.Generator
	auditDestination string
	importMaxItems   int
}

// New constructs the allocation management use case with product guardrails.
func New(pool repo.Pool, allocation repo.CreditAllocation, trusted repo.TrustedMobile, clk clock.Clock, installments, monthly initialcreditlimit.Policy, outbox repo.Outbox, idGenerator identifier.Generator, auditDestination string, importMaxItems int) (*UseCase, error) {
	if pool == nil || allocation == nil || trusted == nil || clk == nil || installments == nil || monthly == nil || outbox == nil || idGenerator == nil || auditDestination == "" {
		return nil, fmt.Errorf("creditallocationuc.New: all dependencies are required")
	}
	if importMaxItems < 1 {
		return nil, fmt.Errorf("creditallocationuc.New: import item limit must be positive")
	}
	return &UseCase{pool: pool, allocation: allocation, trusted: trusted, clock: clk, installments: installments, monthly: monthly, outbox: outbox, idGenerator: idGenerator, auditDestination: auditDestination, importMaxItems: importMaxItems}, nil
}

func (uc *UseCase) tx(ctx context.Context, write bool, fn func(context.Context, repo.Tx, repo.CreditAllocationTxQueryer) error) error {
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := uc.allocation.Tx(tx)
			if err := q.LockConfiguration(ctx, write); err != nil {
				return err
			}
			return fn(ctx, tx, q)
		})
	})
}
