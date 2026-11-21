package config

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	customerext "github.com/parstis/bnpl-backend/pkg/adapter/external/customer"
	circuitbreakernoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
)

func TestExternalCustomer_NewGRPCClient_UnavailableAddressStillStarts(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	cfg := ExternalCustomer{
		Provider: string(customerext.GRPCProvider),
		GRPC: ExternalCustomerGRPC{
			Address:           address,
			Timeout:           time.Second,
			ConnectionTimeout: 200 * time.Millisecond,
			Insecure:          true,
		},
	}

	started := time.Now()
	resolver, closer, err := cfg.newClient(circuitbreakernoop.New(), lognoop.New())
	elapsed := time.Since(started)

	require.NoError(t, err)
	require.NotNil(t, resolver)
	require.NotNil(t, closer)
	require.Less(t, elapsed, 5*time.Second)
	require.NoError(t, closer.Close())
}
