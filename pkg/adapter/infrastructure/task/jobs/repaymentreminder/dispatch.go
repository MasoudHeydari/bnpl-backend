package repaymentreminder

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentreminderuc"
)

// Dispatch returns a job that evaluates repayment reminder eligibility and
// persists Notification Service SMS requests through the transactional outbox.
func Dispatch(
	logger corelog.Logger,
	uc *repaymentreminderuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		out, err := uc.Dispatch(ctx)
		if err != nil {
			logger.Error(
				"repayment reminder dispatch failed",
				"error", err.Error(),
				"candidate_count", out.CandidateCount,
				"created_count", out.CreatedCount,
			)
			return err
		}

		logger.Info(
			"repayment reminder dispatch completed",
			"candidate_count", out.CandidateCount,
			"eligible_customer_count", out.EligibleCustomerCount,
			"created_count", out.CreatedCount,
			"skipped_count", out.SkippedCount,
		)

		return nil
	}
}
