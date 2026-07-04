package merchantterminal

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
)

// defaultScrubBatchSize bounds how many expired issuance deliveries are
// scrubbed in one task tick.
const defaultScrubBatchSize = 100

// IssuanceDeliveryCleanup returns a task job that scrubs ciphertext from
// expired merchant terminal issuance deliveries, leaving idempotency tombstones.
func IssuanceDeliveryCleanup(
	logger corelog.Logger,
	uc *merchantterminaluc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		scrubbed, err := uc.ScrubExpiredIssuanceDeliveries(ctx, defaultScrubBatchSize)
		if err != nil {
			return err
		}

		logger.Info(
			"scrubbed expired merchant terminal issuance deliveries",
			"scrubbed_count", scrubbed,
		)

		return nil
	}
}
