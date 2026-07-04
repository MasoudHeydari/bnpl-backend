package audit

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// EnvelopeIDsFromContext returns correlation_id and trace_id from ctx for the
// audit event envelope.
//
// Request-scoped audited actions must carry both values. The Resource layer
// always places them on the request context before use cases run.
func EnvelopeIDsFromContext(ctx context.Context) (correlationID string, traceID string, err error) {
	const op = "audit.EnvelopeIDsFromContext"

	corr := trace.CorrelationID(ctx)
	if corr == "" {
		return "", "", fmt.Errorf("%s: correlation_id is required in request context", op)
	}

	tid := trace.ID(ctx)
	if tid == "" {
		return "", "", fmt.Errorf("%s: trace_id is required in request context", op)
	}

	return corr, tid, nil
}
