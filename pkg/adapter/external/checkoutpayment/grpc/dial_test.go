package grpc_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	checkoutpaymentgrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment/grpc"
)

func TestDial_RequiresAddress(t *testing.T) {
	t.Parallel()

	_, err := checkoutpaymentgrpc.Dial("", true, checkoutpaymentgrpc.TLSConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "address is required")
}

func TestDial_InsecureSucceedsAgainstLocalListenerShape(t *testing.T) {
	t.Parallel()

	// Dial itself only constructs the client; connection readiness is Ping's job.
	conn, err := checkoutpaymentgrpc.Dial("127.0.0.1:1", true, checkoutpaymentgrpc.TLSConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
}

func TestDial_TLSRequiresPairedClientCertAndKey(t *testing.T) {
	t.Parallel()

	_, err := checkoutpaymentgrpc.Dial("127.0.0.1:1", false, checkoutpaymentgrpc.TLSConfig{
		CertFile: "only-cert.pem",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cert_file and key_file must both be set")
}

func TestDial_TLSAcceptsCAFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(caFile, mustSelfSignedCAPEM(t), 0o600))

	conn, err := checkoutpaymentgrpc.Dial("127.0.0.1:1", false, checkoutpaymentgrpc.TLSConfig{
		CAFile:     caFile,
		ServerName: "financial-core-service",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
}

func mustSelfSignedCAPEM(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
