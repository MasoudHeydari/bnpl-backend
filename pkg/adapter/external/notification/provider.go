package notification

// Provider identifies a Notification Service client implementation.
type Provider string

const (
	// GRPCProvider uses the Notification Service over gRPC.
	GRPCProvider Provider = "grpc"

	// NoopProvider accepts submissions without calling an external service.
	// It is never allowed in production.
	NoopProvider Provider = "noop"
)
