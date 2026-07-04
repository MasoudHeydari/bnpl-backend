package noop

import (
	"context"

	adapterbotmessage "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage"
	corebotmessage "github.com/parstis/bnpl-backend/pkg/core/external/botmessage"
)

// Client keeps bot messaging wiring available without contacting an external
// provider. It is intended for local/test environments where bot I/O is not
// required.
type Client struct{}

var _ adapterbotmessage.Integration = (*Client)(nil)

// New constructs the noop bot messaging integration.
func New() *Client {
	return &Client{}
}

// ID returns the stable noop integration routing identifier.
func (c *Client) ID() adapterbotmessage.IntegrationID {
	return adapterbotmessage.NoopIntegrationID
}

// Send accepts the message without external delivery.
func (c *Client) Send(_ context.Context, _ corebotmessage.SendInput) (corebotmessage.SendResult, error) {
	return corebotmessage.SendResult{}, nil
}

// Run blocks until cancellation and emits no inbound messages.
func (c *Client) Run(ctx context.Context, _ corebotmessage.IncomingMessageHandler) error {
	<-ctx.Done()
	return nil
}
