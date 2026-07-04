package checkoutsession

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// RecoverConfirming returns a task job that relinks persisted Contracts and
// resumes interrupted FC recognition without releasing unfenced claims.
func RecoverConfirming(
	logger corelog.Logger,
	uc *checkoutsessionuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.RecoverStaleConfirmingSessions(ctx)
		if err != nil {
			return err
		}

		logger.Info(
			"reconciled interrupted checkout sessions",
			"relinked_count",
			output.RelinkedCount,
			"resumed_count",
			output.ResumedCount,
			"unresolved_count",
			output.UnresolvedCount,
		)

		return nil
	}
}
