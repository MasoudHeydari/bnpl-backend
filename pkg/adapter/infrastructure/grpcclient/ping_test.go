package grpcclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/grpcclient"
)

func TestPing_NilConnection(t *testing.T) {
	t.Parallel()

	err := grpcclient.Ping(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection is nil")
}

func TestPing_ReadyLocalServer(t *testing.T) {
	t.Parallel()

	lis := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	go server.Serve(lis)
	t.Cleanup(server.Stop)

	ctx := context.Background()
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	pingCtx, cancel := context.WithTimeout(ctx, grpcclient.ConnectTimeout)
	defer cancel()

	require.NoError(t, grpcclient.Ping(pingCtx, conn))
}

func TestPing_UnavailableAddress(t *testing.T) {
	t.Parallel()

	// Bind then close so the port is locally unused.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = grpcclient.Ping(pingCtx, conn)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestPing_ContextDeadline(t *testing.T) {
	t.Parallel()

	conn, err := grpc.NewClient(
		"127.0.0.1:1",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	err = grpcclient.Ping(pingCtx, conn)
	elapsed := time.Since(started)

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, elapsed, 2*time.Second)
}
