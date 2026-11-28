package grpc

import (
	"fmt"
	"strings"

	grpcclient "google.golang.org/grpc"

	adaptergrpc "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/grpcclient"
)

type TLSConfig = adaptergrpc.TLSConfig

// Dial requires mTLS; plaintext or server-only TLS is not a valid transport.
func Dial(address string, tlsCfg TLSConfig) (*grpcclient.ClientConn, error) {
	const op = "grpc.Dial"

	if strings.TrimSpace(tlsCfg.CAFile) == "" ||
		strings.TrimSpace(tlsCfg.CertFile) == "" ||
		strings.TrimSpace(tlsCfg.KeyFile) == "" ||
		strings.TrimSpace(tlsCfg.ServerName) == "" {
		return nil, fmt.Errorf("%s: Merchant mTLS requires CA, certificate, key and server name", op)
	}

	return adaptergrpc.Dial(address, false, tlsCfg)
}
