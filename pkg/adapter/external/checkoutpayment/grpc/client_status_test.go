package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"

	paymentv1 "github.com/parstis/bnpl-backend/gen/proto/payment/v1"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

func TestMapCheckoutPaymentStatus_CancelledProtoMapsToCanonicalCanceled(t *testing.T) {
	t.Parallel()

	status, err := mapCheckoutPaymentStatus(
		paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_CANCELLED,
	)
	require.NoError(t, err)
	require.Equal(t, extcheckoutpayment.CanceledPaymentStatus, status)
	require.Equal(t, extcheckoutpayment.PaymentStatus("canceled"), status)
}
