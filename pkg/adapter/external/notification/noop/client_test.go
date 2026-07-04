package noop

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
)

func TestSubmit_AcceptsRequest(t *testing.T) {
	t.Parallel()

	out, err := New().Submit(context.Background(), extnotification.SubmitInput{
		TemplateID:     "bnpl.external_checkout_otp.v1",
		Recipient:      "+989121234567",
		IdempotencyKey: "checkout-otp:cs:oi",
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
		Variables: map[string]string{
			"code":          "012345",
			"merchant_name": "Shop",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.RequestID)
	require.False(t, out.Duplicate)
}
