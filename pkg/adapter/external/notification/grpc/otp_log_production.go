//go:build production

package grpc

import (
	"context"

	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
)

// logOutboundOTP is a no-op in production builds so plaintext OTP never lands
// in application logs.
func (c *Client) logOutboundOTP(context.Context, extnotification.SubmitInput) {}
