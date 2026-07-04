//go:build e2e

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

func TestE2E_FCFake_CreateCheckoutPaymentRequiresExpiresAt(t *testing.T) {
	t.Parallel()

	fc := newControllableCheckoutPaymentClient()
	ctx := context.Background()

	base := extcheckoutpayment.CreateCheckoutPaymentInput{
		IdempotencyKey:         "e2e-idem-expires",
		SourceType:             "bnpl_payment_attempt",
		SourceID:               "attempt-expires-1",
		PayerType:              extcheckoutpayment.PartyCustomer,
		PayerID:                "customer-1",
		PayeeType:              extcheckoutpayment.PartyPlatform,
		PayeeID:                "",
		AmountMinor:            20,
		Currency:               "IRR",
		CollectionObligationID: "co-1",
		SuccessReturnURL:       "https://bnpl.example/return",
		FailureReturnURL:       "https://bnpl.example/return",
		CancelReturnURL:        "https://bnpl.example/return",
		Description:            "BNPL installment repayment",
	}

	_, err := fc.CreateCheckoutPayment(ctx, base)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expires_at is required")
	require.Equal(t, 0, fc.committedPaymentCount())

	withExpiry := base
	withExpiry.ExpiresAt = time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC)
	got, err := fc.CreateCheckoutPayment(ctx, withExpiry)
	require.NoError(t, err)
	require.True(t, got.ExpiresAt.UTC().Equal(withExpiry.ExpiresAt.UTC()))
	require.Equal(t, 1, fc.committedPaymentCount())

	// Idempotent replay must keep the same financial identity / expiry.
	replay, err := fc.CreateCheckoutPayment(ctx, withExpiry)
	require.NoError(t, err)
	require.True(t, replay.ReusedIdempotentResult)
	require.Equal(t, got.PaymentIntentID, replay.PaymentIntentID)
	require.True(t, replay.ExpiresAt.UTC().Equal(withExpiry.ExpiresAt.UTC()))
	require.Equal(t, 1, fc.committedPaymentCount())
}
