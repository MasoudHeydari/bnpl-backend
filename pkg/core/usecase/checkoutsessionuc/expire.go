package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ExpireDueOpenSessions expires open checkout sessions whose handoff lifetime
// elapsed and enqueues merchant lifecycle events.
//
// PROCESSING sessions are never expired here: an unresolved Financial Core
// payment outcome outranks the session TTL. Open sessions with accepted
// Contracts are also exempt after payment failure: a late FC success must not
// be converted into an expired (and potentially re-financed) purchase.
func (uc *UseCase) ExpireDueOpenSessions(ctx context.Context) (ExpireDueOpenSessionsOutput, error) {
	const op = "checkoutsessionuc.UseCase.ExpireDueOpenSessions"

	var output ExpireDueOpenSessionsOutput

	now := uc.clock.Now().UTC()

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)

					expired, expireErr := sessionQ.ExpireOpenDue(ctx, now, uc.expiryBatchSize)
					if expireErr != nil {
						return fmt.Errorf("%s: expire open due sessions: %w", op, expireErr)
					}

					for _, session := range expired {
						if enqueueErr := uc.lifecycle.Enqueue(
							ctx, tx, session, checkoutlifecycle.EventTypeExpired, now,
						); enqueueErr != nil {
							return enqueueErr
						}
					}

					output.ExpiredCount = len(expired)

					return nil
				},
			)
		},
	)
	if err != nil {
		return ExpireDueOpenSessionsOutput{}, err
	}

	return output, nil
}
