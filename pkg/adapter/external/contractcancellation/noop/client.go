package noop

import (
	"context"

	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
)

// Client is a local-development placeholder for Financial Core contract
// cancellation. It never pretends a financial cancellation succeeded.
type Client struct{}

var _ extcontractcancellation.Client = (*Client)(nil)

// New constructs a noop contract cancellation client.
func New() *Client {
	return &Client{}
}

// RequestCancellation refuses to fabricate Financial Core cancellation success.
func (c *Client) RequestCancellation(
	_ context.Context,
	_ extcontractcancellation.RequestCancellationInput,
) (extcontractcancellation.RequestCancellationResult, error) {
	return extcontractcancellation.RequestCancellationResult{},
		extcontractcancellation.Permanent(extcontractcancellation.ErrNotSupported)
}
