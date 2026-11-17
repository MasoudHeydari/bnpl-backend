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
			logger.With(
				"error",
				err.Error(),
				"candidate_count",
				out.CandidateCount,
				"created_count",
				out.CreatedCount,
			).Error(
				corelog.Msg("repayment reminder dispatch failed"),
			)
			return err
		}

		logger.With(
			"candidate_count",
			out.CandidateCount,
			"eligible_customer_count",
			out.EligibleCustomerCount,
			"created_count",
			out.CreatedCount,
			"skipped_count",
			out.SkippedCount,
		).Info(
			corelog.Msg("repayment reminder dispatch completed"),
		)

		return nil
	}
}
