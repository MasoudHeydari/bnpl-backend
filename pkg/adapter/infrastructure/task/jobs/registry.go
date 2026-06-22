// Package jobs resolves configured task handlers to executable jobs.
package jobs

import (
	"fmt"

	runtimejob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/runtime"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
)

// Registry resolves task handlers to executable jobs.
type Registry struct {
	logger corelog.Logger
}

// NewRegistry creates a task job registry.
func NewRegistry(logger corelog.Logger) Registry {
	return Registry{
		logger: logger,
	}
}

// Resolve resolves a configured handler to an executable job.
func (r Registry) Resolve(handler string) (coretask.Job, error) {
	switch handler {
	case "runtime_goroutine_metrics":
		return runtimejob.GoroutineMetrics(r.logger), nil

	default:
		return nil, fmt.Errorf("unknown task handler: %q", handler)
	}
}
