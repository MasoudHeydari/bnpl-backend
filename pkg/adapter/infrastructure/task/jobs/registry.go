package jobs

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/outbox"
	runtimejob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/runtime"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
)

// Registry resolves task handlers to executable jobs.
type Registry struct {
	logger            corelog.Logger
	outboxpublisheruc *outboxpublisheruc.UseCase
}

// NewRegistry creates a task job registry.
func NewRegistry(
	logger corelog.Logger,
	outboxpublisheruc *outboxpublisheruc.UseCase,
) Registry {
	return Registry{
		logger:            logger,
		outboxpublisheruc: outboxpublisheruc,
	}
}

// Resolve resolves a configured handler to an executable job.
func (r Registry) Resolve(handler string) (coretask.Job, error) {
	switch handler {
	case "runtime_goroutine_metrics":
		return runtimejob.GoroutineMetrics(r.logger), nil

	case "outbox_publisher":
		return outbox.Publisher(r.outboxpublisheruc), nil

	default:
		return nil, fmt.Errorf("unknown task handler: %q", handler)
	}
}
