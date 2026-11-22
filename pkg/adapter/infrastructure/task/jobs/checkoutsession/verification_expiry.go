package checkoutsession

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// VerificationExpiry returns a task job that moves due verify_pending
// checkouts to cancellation_pending without releasing credit exposure.
func VerificationExpiry(
	logger corelog.Logger,
	uc *checkoutsessionuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.ExpirePendingMerchantVerifications(ctx)
		if err != nil {
			return err
		}

		logger.With(
			"claimed_count",
			output.Claimed,
			"expired_count",
			output.Expired,
		).Info(
			corelog.Msg("expired due merchant verification windows"),
		)

		return nil
	}
}
