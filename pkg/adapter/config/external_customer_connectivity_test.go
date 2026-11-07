package config

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	customerext "github.com/parstis/bnpl-backend/pkg/adapter/external/customer"
	circuitbreakernoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
)

func TestExternalCustomer_NewGRPCClient_UnavailableAddressFailsFast(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	cfg := ExternalCustomer{
		Provider: string(customerext.GRPCProvider),
		GRPC: ExternalCustomerGRPC{
			Address:  address,
			Timeout:  time.Second,
			Insecure: true,
		},
	}

	started := time.Now()
	resolver, closer, err := cfg.newClient(circuitbreakernoop.New())
	elapsed := time.Since(started)

	require.Error(t, err)
	require.Nil(t, resolver)
	require.Nil(t, closer)
	require.Contains(t, err.Error(), "customer grpc service")
	require.Contains(t, err.Error(), address)
	require.Contains(t, err.Error(), "unavailable")
	require.Less(t, elapsed, 5*time.Second)
}
