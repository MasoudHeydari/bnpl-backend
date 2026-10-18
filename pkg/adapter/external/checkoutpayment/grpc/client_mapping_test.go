package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

func TestCreateCheckoutPayment_CustomerPayerFailsClosedWithoutDelegation(t *testing.T) {
	t.Parallel()

	client := &Client{timeout: time.Second}
	_, err := client.CreateCheckoutPayment(context.Background(), extcheckoutpayment.CreateCheckoutPaymentInput{
		PayerType:              extcheckoutpayment.PartyCustomer,
		PayerID:                "11111111-1111-4111-8111-111111111111",
		CollectionObligationID: "co-1",
		AmountMinor:            100,
		Currency:               "IRR",
		SourceType:             "bnpl_contract",
		SourceID:               "contract-1",
		IdempotencyKey:         "idem-1",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, extcheckoutpayment.ErrFinancialDelegationUnavailable))
	require.NotContains(t, err.Error(), "Bearer")
	require.NotContains(t, stringsToLower(err.Error()), "eyj")
}

func TestMapCollectionApplication_PreRecognitionCash(t *testing.T) {
	t.Parallel()

	app, err := mapCollectionApplication(extcheckoutpayment.PrepareCollectionObligationInput{
		Application: extcheckoutpayment.PreRecognitionCashCollectionApplication,
	})
	require.NoError(t, err)
	require.NotNil(t, app.GetPreRecognitionCash())
	require.Nil(t, app.GetExistingReceivable())
}

func TestMapCollectionApplication_ExistingReceivableRequiresMerchantEntitlementID(t *testing.T) {
	t.Parallel()

	_, err := mapCollectionApplication(extcheckoutpayment.PrepareCollectionObligationInput{
		Application: extcheckoutpayment.ExistingReceivableCollectionApplication,
	})
	require.Error(t, err)

	app, err := mapCollectionApplication(extcheckoutpayment.PrepareCollectionObligationInput{
		Application:           extcheckoutpayment.ExistingReceivableCollectionApplication,
		MerchantEntitlementID: "me-1",
	})
	require.NoError(t, err)
	require.Equal(t, "me-1", app.GetExistingReceivable().GetMerchantEntitlementId())
}

func stringsToLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
