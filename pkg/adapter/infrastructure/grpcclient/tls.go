package grpcclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TLSConfig carries optional TLS materials for outbound gRPC dial.
//
// Used when insecure transport is false:
//   - CAFile, when set, replaces the system trust store for server verification
//   - CertFile + KeyFile enable mTLS client authentication
//   - ServerName overrides certificate hostname verification (needed when
//     dialing by IP while the server certificate uses a DNS CN/SAN)
type TLSConfig struct {
	CAFile     string
	CertFile   string
	KeyFile    string
	ServerName string
}

// Dial opens a gRPC client connection with either plaintext or TLS/mTLS.
func Dial(address string, insecureTransport bool, tlsCfg TLSConfig) (*grpc.ClientConn, error) {
	const op = "grpcclient.Dial"

	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("%s: address is required", op)
	}

	transport, err := newTransportCredentials(insecureTransport, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(transport))
	if err != nil {
		return nil, fmt.Errorf("%s: dial %q: %w", op, address, err)
	}

	return conn, nil
}

func newTransportCredentials(
	insecureTransport bool,
	cfg TLSConfig,
) (credentials.TransportCredentials, error) {
	const op = "grpcclient.newTransportCredentials"

	if insecureTransport {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: strings.TrimSpace(cfg.ServerName),
	}

	if caFile := strings.TrimSpace(cfg.CAFile); caFile != "" {
		pemBytes, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("%s: read ca_file: %w", op, err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("%s: ca_file contains no valid certificates", op)
		}

		tlsConfig.RootCAs = pool
	}

	certFile := strings.TrimSpace(cfg.CertFile)
	keyFile := strings.TrimSpace(cfg.KeyFile)
	switch {
	case certFile == "" && keyFile == "":
		// Server TLS only (no client certificate).

	case certFile == "" || keyFile == "":
		return nil, fmt.Errorf("%s: cert_file and key_file must both be set for mTLS", op)

	default:
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("%s: load client certificate: %w", op, err)
		}

		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(tlsConfig), nil
}
