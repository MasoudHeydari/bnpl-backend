package grpc

import (
	"fmt"
	"strings"

	grpcclient "google.golang.org/grpc"

	adaptergrpc "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/grpcclient"
)

// TLSConfig carries optional TLS materials for Customer Reference gRPC dial.
type TLSConfig = adaptergrpc.TLSConfig

// Dial opens a gRPC client connection to the Customer Reference Service.
func Dial(address string, insecureTransport bool, tlsCfg TLSConfig) (*grpcclient.ClientConn, error) {
	const op = "grpc.Dial"

	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("%s: customer grpc address is required", op)
	}

	conn, err := adaptergrpc.Dial(address, insecureTransport, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("%s: dial customer grpc service: %w", op, err)
	}

	return conn, nil
}
