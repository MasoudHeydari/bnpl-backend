package merchantterminaluc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ScrubExpiredIssuanceDeliveries purges the replayable secret of up to
// batchSize expired issuance deliveries and returns how many were scrubbed.
//
// Scrubbing clears the ciphertext and its key id and records purged_at. The row
// itself stays as an idempotency tombstone, so a reused Idempotency-Key still
// cannot mint a second API key. A worker should call this repeatedly until it
// returns fewer rows than batchSize.
func (uc *UseCase) ScrubExpiredIssuanceDeliveries(ctx context.Context, batchSize int) (int64, error) {
	const op = "merchantterminaluc.UseCase.ScrubExpiredIssuanceDeliveries"

	if batchSize <= 0 {
		return 0, fmt.Errorf("%s: batch size must be positive", op)
	}

	var scrubbed int64

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					terminalQ := uc.merchantTerminal.Tx(tx)
					now := uc.clock.Now().UTC()

					affected, err := terminalQ.ScrubExpiredIssuanceDeliveries(ctx, now, batchSize)
					if err != nil {
						return fmt.Errorf("%s: scrub deliveries: %w", op, err)
					}

					scrubbed = affected

					return nil
				},
			)
		},
	)
	if err != nil {
		return 0, err
	}

	return scrubbed, nil
}
