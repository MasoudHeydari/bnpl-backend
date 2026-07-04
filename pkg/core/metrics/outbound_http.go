package metrics

import (
	"context"
	"time"
)

// OutboundHTTPMetrics records operational behavior of the shared outbound HTTP
// infrastructure.
//
// Implementations must use only bounded, low-cardinality dimensions. Raw URLs,
// URL paths, query strings, trace/request/correlation IDs, entity IDs, and
// other effectively unbounded values must never become metric labels.
type OutboundHTTPMetrics interface {
	// OutboundHTTPInFlightChanged adjusts the current number of requests holding
	// an outbound execution slot by delta.
	OutboundHTTPInFlightChanged(ctx context.Context, delta int)

	// OutboundHTTPQueueDepthChanged adjusts the current number of callers waiting
	// in the bounded outbound queue by delta.
	OutboundHTTPQueueDepthChanged(ctx context.Context, delta int)

	// OutboundHTTPQueueWaitObserved records how long a queued request waited for
	// an execution slot.
	OutboundHTTPQueueWaitObserved(ctx context.Context, duration time.Duration)

	// OutboundHTTPRequestCompleted records one completed outbound attempt using
	// bounded method and outcome values plus its observed duration.
	OutboundHTTPRequestCompleted(ctx context.Context, method, outcome string, duration time.Duration)

	// OutboundHTTPSaturated records a request rejected because both execution and
	// queue capacity were exhausted.
	OutboundHTTPSaturated(ctx context.Context)
}
