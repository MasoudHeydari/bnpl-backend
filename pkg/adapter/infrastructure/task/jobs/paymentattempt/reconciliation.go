package paymentattempt

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
)

// Reconciliation returns a task job that executes one payment attempt reconciliation cycle.
func Reconciliation(
	uc *paymentattemptreconciliationuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		return uc.ProcessDue(ctx)
	}
}
