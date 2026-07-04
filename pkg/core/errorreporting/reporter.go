package errorreporting

import "context"

// Metadata contains bounded diagnostic data attached to one reported failure.
//
// Callers must only add non-sensitive values. Request/response bodies, headers,
// cookies, credentials, raw messaging payloads, and other PII must never be put
// in Tags or Contexts.
type Metadata struct {
	// Component identifies the runtime boundary that observed the failure, for
	// example "http" or "task".
	Component string

	// Tags contains low-cardinality searchable dimensions.
	Tags map[string]string

	// Contexts contains structured, non-indexed diagnostic values. High-cardinality
	// correlation identifiers belong here rather than in Tags.
	Contexts map[string]map[string]any

	// Fingerprint optionally overrides provider-side issue grouping.
	Fingerprint []string
}

// Reporter reports unexpected failures without participating in business
// correctness. Implementations must be best-effort: an unavailable reporting
// backend must never make a request, consumer, task, or shutdown fail.
type Reporter interface {
	// CaptureError reports an unexpected error.
	CaptureError(ctx context.Context, err error, metadata Metadata)

	// CapturePanic reports a recovered panic. The reporter never owns control-flow
	// policy; runtime boundaries decide how the failed execution is contained or retried.
	CapturePanic(ctx context.Context, recovered any, metadata Metadata)

	// Flush waits until buffered events are delivered or ctx expires.
	// A false result means delivery could not be confirmed within the deadline.
	Flush(ctx context.Context) bool

	// Close releases reporter-owned resources. Close must not make application
	// shutdown fail because an observability backend is unavailable.
	Close() error
}
