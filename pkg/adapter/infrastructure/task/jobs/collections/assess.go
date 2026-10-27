package collections

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/collectionsuc"
)

// Assess runs periodic collections default/recovery assessment.
func Assess(
	logger log.Logger,
	uc *collectionsuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		const op = "jobs.collections.Assess"

		output, err := uc.AssessBatch(ctx)
		logger.With(
			"assessed", output.AssessedCount,
			"defaulted", output.DefaultedCount,
			"recovered", output.RecoveredCount,
			"skipped", output.SkippedCount,
			"failed", output.FailedCount,
		).Info("collections assessment completed")

		if err != nil {
			logger.With(
				"error", err.Error(),
				"assessed", output.AssessedCount,
				"failed", output.FailedCount,
			).Error("collections assessment had customer failures")
			return fmt.Errorf("%s: %w", op, err)
		}

		return nil
	}
}
