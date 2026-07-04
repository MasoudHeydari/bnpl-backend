package checkoutpayment

// Provider identifies a Financial Core checkout payment client implementation.
type Provider string

const (
	// GRPCProvider uses Financial Core over gRPC.
	GRPCProvider Provider = "grpc"

	// NoopProvider returns a deterministic local checkout payment result without
	// calling an external service.
	NoopProvider Provider = "noop"
)
