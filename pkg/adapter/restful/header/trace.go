package header

const (
	// TraceID carries the identifier of a logical execution flow.
	//
	// The same trace ID may span multiple requests, service calls, database
	// operations and asynchronous messages that belong to the same business
	// operation.
	TraceID = "X-Trace-Id"

	// RequestID uniquely identifies a single inbound HTTP request.
	//
	// A new request ID is typically created at the transport boundary when one
	// is not supplied by the caller.
	RequestID = "X-Request-Id"

	// CorrelationID links related work across process or service boundaries.
	//
	// It is commonly propagated through HTTP calls, message brokers and
	// background jobs to correlate distributed operations.
	CorrelationID = "X-Correlation-Id"
)
