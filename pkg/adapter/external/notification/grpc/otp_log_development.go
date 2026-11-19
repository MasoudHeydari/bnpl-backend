//go:build development

package grpc

import (
	"context"

	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/log"
)

// logOutboundOTP records the plaintext OTP for local delivery debugging.
func (c *Client) logOutboundOTP(ctx context.Context, input extnotification.SubmitInput) {
	code := ""
	if input.Variables != nil {
		code = input.Variables["code"]
	}

	c.logger.WithContext(ctx).With(
		"template_id", input.TemplateID,
		"idempotency_key", input.IdempotencyKey,
		"otp_code", code,
	).Info(log.Msg("otp notification submit: plaintext code for local delivery debugging"))
}
