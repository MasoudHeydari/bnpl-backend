package outbox

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
)

// Publisher returns a task job that executes one outbox publishing cycle.
func Publisher(
	uc *outboxpublisheruc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		return uc.PublishDue(ctx)
	}
}
