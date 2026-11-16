package runtime

import (
	"context"
	"runtime"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

// GoroutineMetrics returns a job that logs the current goroutine count.
func GoroutineMetrics(logger corelog.Logger) task.Job {
	return func(ctx context.Context) error {
		logger.With(
		"goroutine_count",
		runtime.NumGoroutine(),
	).Info(
		corelog.Msg("runtime goroutine metrics"),
	)

		return nil
	}
}
