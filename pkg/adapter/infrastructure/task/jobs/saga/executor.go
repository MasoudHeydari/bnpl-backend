package saga

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/sagaexecutoruc"
)

// Executor returns a background job that resumes due durable Saga instances.
func Executor(
	logger corelog.Logger,
	uc *sagaexecutoruc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		if err := uc.ProcessDue(ctx); err != nil {
			return err
		}

		logger.Info("processed due saga instances")
		return nil
	}
}
