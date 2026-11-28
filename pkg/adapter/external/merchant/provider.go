package merchant

// Provider identifies the transport implementation.
type Provider string

const (
	// NoopProvider disables the remote integration without fabricating results.
	NoopProvider Provider = "noop"
	// GRPCProvider selects the private mTLS gRPC contract.
	GRPCProvider Provider = "grpc"
)
