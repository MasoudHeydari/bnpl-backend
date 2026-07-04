package paymentattempt

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptexpirationuc"
)

// Expiration returns a task job that expires local CreateCheckoutPayment windows.
func Expiration(
	uc *paymentattemptexpirationuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		return uc.ProcessDue(ctx)
	}
}
