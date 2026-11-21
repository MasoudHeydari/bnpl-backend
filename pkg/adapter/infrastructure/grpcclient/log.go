package grpcclient

import (
	"github.com/parstis/bnpl-backend/pkg/core/log"
)

// LogUnavailable records that an outbound gRPC dependency could not be reached
// for this request. Callers still return a public Bad Gateway error; this log
// identifies which service failed for operators.
func LogUnavailable(logger log.Logger, service, op string, cause error) {
	if logger == nil || cause == nil {
		return
	}

	logger.With(
		"service", service,
		"op", op,
		"error", cause.Error(),
	).Warn(log.Msg("external service unavailable"))
}
