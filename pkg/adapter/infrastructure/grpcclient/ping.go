package grpcclient

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// ConnectTimeout is the startup budget for verifying a configured gRPC
// dependency reaches READY after grpc.NewClient.
const ConnectTimeout = 3 * time.Second

// Ping triggers connection establishment on conn and waits until the transport
// reaches connectivity.Ready or ctx ends.
//
// It does not invoke business RPCs and does not require the gRPC health
// protocol. A nil connection is rejected immediately.
func Ping(ctx context.Context, conn *grpc.ClientConn) error {
	const op = "grpcclient.Ping"

	if conn == nil {
		return fmt.Errorf("%s: connection is nil", op)
	}

	conn.Connect()

	for {
		state := conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil

		case connectivity.Shutdown:
			return fmt.Errorf("%s: connection is shut down", op)
		}

		if !conn.WaitForStateChange(ctx, state) {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}

			return fmt.Errorf("%s: wait for state change interrupted", op)
		}
	}
}
