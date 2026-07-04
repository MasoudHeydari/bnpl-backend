package noop

import (
	"context"
	"fmt"
	"strings"

	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
)

var _ extnotification.Submitter = (*Client)(nil)

// Client accepts notification submissions without calling the Notification
// Service.
type Client struct{}

// New constructs a noop Notification Service client.
func New() *Client {
	return &Client{}
}

// Submit returns a deterministic accepted result without external I/O.
func (c *Client) Submit(
	_ context.Context,
	input extnotification.SubmitInput,
) (extnotification.SubmitOutput, error) {
	const op = "noop.Client.Submit"

	if strings.TrimSpace(input.Recipient) == "" {
		return extnotification.SubmitOutput{}, fmt.Errorf("%s: recipient is required", op)
	}

	if strings.TrimSpace(input.TemplateID) == "" {
		return extnotification.SubmitOutput{}, fmt.Errorf("%s: template id is required", op)
	}

	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return extnotification.SubmitOutput{}, fmt.Errorf("%s: idempotency key is required", op)
	}

	if input.ExpiresAt.IsZero() {
		return extnotification.SubmitOutput{}, fmt.Errorf("%s: expires_at is required", op)
	}

	return extnotification.SubmitOutput{
		RequestID: "noop-notification-" + input.IdempotencyKey,
		Duplicate: false,
	}, nil
}
