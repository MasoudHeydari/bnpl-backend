package grpcclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// Ping triggers connection establishment on conn and waits until the transport
// reaches connectivity.Ready or ctx ends.
//
// Callers must bound ctx with their configured connection timeout. Ping does
// not invoke business RPCs and does not require the gRPC health protocol. A
// nil connection is rejected immediately.
func Ping(ctx context.Context, conn *grpc.ClientConn) error {
	const op = "grpcclient.Ping"

	if conn == nil {
		return fmt.Errorf("%s: connection is nil", op)
	}

	conn.Connect()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}

		if state == connectivity.Shutdown {
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
