package checkoutsession

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// Expiry returns a task job that expires due open checkout sessions.
func Expiry(
	logger corelog.Logger,
	uc *checkoutsessionuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.ExpireDueOpenSessions(ctx)
		if err != nil {
			return err
		}

		logger.With(
		"expired_count",
		output.ExpiredCount,
	).Info(
		corelog.Msg("expired due open checkout sessions"),
	)

		return nil
	}
}
