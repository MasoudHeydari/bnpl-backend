package identity

// Provider identifies an Identity Service client implementation.
type Provider string

const (
	// GRPCProvider uses the Identity Service over gRPC.
	GRPCProvider Provider = "grpc"

	// NoopProvider resolves mobiles to deterministic local subjects without
	// calling an external service. It is never allowed in production.
	NoopProvider Provider = "noop"
)
