package customer

// Provider identifies a Customer Reference Service client implementation.
type Provider string

const (
	// GRPCProvider uses the Customer Reference Service over gRPC.
	GRPCProvider Provider = "grpc"

	// NoopProvider resolves mobiles to deterministic local customers without
	// calling an external service. It is never allowed in production.
	NoopProvider Provider = "noop"
)
