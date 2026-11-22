package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ExpirePendingMerchantVerificationsOutput summarizes one timeout sweep.
type ExpirePendingMerchantVerificationsOutput struct {
	Claimed int
	Expired int
}

// ExpirePendingMerchantVerifications moves due verify_pending checkouts to
// cancellation_pending without releasing credit or cancelling contracts.
//
// TODO: Integrate Financial Core refund/reversal for merchant verification
// timeout. Keep checkout cancellation_pending until authoritative financial
// compensation is completed; never release credit exposure prematurely.
func (uc *UseCase) ExpirePendingMerchantVerifications(
	ctx context.Context,
) (ExpirePendingMerchantVerificationsOutput, error) {
	const op = "checkoutsessionuc.UseCase.ExpirePendingMerchantVerifications"

	now := uc.clock.Now().UTC()
	var output ExpirePendingMerchantVerificationsOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)

			claimed, err := sessionQ.ClaimDueVerifyPending(ctx, now, uc.expiryBatchSize)
			if err != nil {
				return fmt.Errorf("%s: claim due verify pending: %w", op, err)
			}

			output.Claimed = len(claimed)

			for _, session := range claimed {
				updated, markErr := sessionQ.MarkCancellationPending(ctx, session.ID, now)
				if markErr != nil {
					if cerr.IsCode(markErr, cerr.CodeConflict) {
						continue
					}

					return markErr
				}

				if err = uc.lifecycle.Enqueue(
					ctx,
					tx,
					updated,
					checkoutlifecycle.EventTypeCancellationPending,
					now,
				); err != nil {
					return err
				}

				output.Expired++
			}

			return nil
		})
	})
	if err != nil {
		return ExpirePendingMerchantVerificationsOutput{}, err
	}

	return output, nil
}
