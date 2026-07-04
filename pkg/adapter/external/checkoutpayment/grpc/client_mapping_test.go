package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

func validCreateCheckoutPaymentInput() extcheckoutpayment.CreateCheckoutPaymentInput {
	return extcheckoutpayment.CreateCheckoutPaymentInput{
		IdempotencyKey:         "idem-1",
		SourceType:             "bnpl_contract",
		SourceID:               "11111111-1111-4111-8111-111111111111",
		PayerType:              extcheckoutpayment.PartyCustomer,
		PayerID:                "22222222-2222-4222-8222-222222222222",
		PayeeType:              extcheckoutpayment.PartyPlatform,
		PayeeID:                "",
		AmountMinor:            1000,
		Currency:               "IRR",
		ProviderCode:           "provider",
		CollectionObligationID: "co-1",
		SuccessReturnURL:       "https://example.test/success",
		FailureReturnURL:       "https://example.test/failure",
		CancelReturnURL:        "https://example.test/cancel",
		Description:            "BNPL checkout",
		ExpiresAt:              time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
	}
}

func TestValidateCreateCheckoutPaymentInput_AcceptsCustomerPlatformEmptyPayee(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateCreateCheckoutPaymentInput(validCreateCheckoutPaymentInput()))
}

func TestValidateCreateCheckoutPaymentInput_RejectsInvalidParties(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*extcheckoutpayment.CreateCheckoutPaymentInput)
	}{
		{
			name: "merchant payer",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerType = extcheckoutpayment.PartyMerchant
			},
		},
		{
			name: "legacy subject payer",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerType = "subject"
			},
		},
		{
			name: "merchant payee",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayeeType = extcheckoutpayment.PartyMerchant
				in.PayeeID = "merchant-1"
			},
		},
		{
			name: "platform payee with merchant id substituted",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayeeID = "merchant-1"
			},
		},
		{
			name: "missing payer id",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerID = ""
			},
		},
		{
			name: "missing collection obligation",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.CollectionObligationID = ""
			},
		},
		{
			name: "missing idempotency key",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.IdempotencyKey = ""
			},
		},
		{
			name: "non-positive amount",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.AmountMinor = 0
			},
		},
		{
			name: "missing expires_at",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.ExpiresAt = time.Time{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validCreateCheckoutPaymentInput()
			tt.mutate(&in)
			err := validateCreateCheckoutPaymentInput(in)
			require.Error(t, err)
			require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
		})
	}
}

func TestCreateCheckoutPayment_RejectsInvalidInputWithoutNetwork(t *testing.T) {
	t.Parallel()

	client := &Client{timeout: time.Second}
	in := validCreateCheckoutPaymentInput()
	in.PayeeType = extcheckoutpayment.PartyMerchant
	in.PayeeID = "merchant-1"

	_, err := client.CreateCheckoutPayment(context.Background(), in)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
	require.NotContains(t, err.Error(), "Bearer")
}

func TestNormalizeCreateCheckoutPaymentInput_CanonicalizesPartyTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payerType string
		payeeType string
		wantPayer string
		wantPayee string
	}{
		{
			name:      "exact canonical",
			payerType: "customer",
			payeeType: "platform",
			wantPayer: "customer",
			wantPayee: "platform",
		},
		{
			name:      "whitespace and casing",
			payerType: " CUSTOMER ",
			payeeType: " Platform ",
			wantPayer: "customer",
			wantPayee: "platform",
		},
		{
			name:      "mixed ascii case",
			payerType: "CuStOmEr",
			payeeType: "PLATFORM",
			wantPayer: "customer",
			wantPayee: "platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validCreateCheckoutPaymentInput()
			in.PayerType = tt.payerType
			in.PayeeType = tt.payeeType
			payerID := in.PayerID

			got, err := normalizeCreateCheckoutPaymentInput(in)
			require.NoError(t, err)
			require.Equal(t, tt.wantPayer, got.PayerType)
			require.Equal(t, tt.wantPayee, got.PayeeType)
			require.Equal(t, payerID, got.PayerID)
			require.Empty(t, got.PayeeID)
		})
	}
}

func TestNormalizeCreateCheckoutPaymentInput_RejectsUnsupportedPartyTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*extcheckoutpayment.CreateCheckoutPaymentInput)
	}{
		{
			name: "legacy subject",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerType = " SUBJECT "
			},
		},
		{
			name: "merchant payer",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerType = "Merchant"
			},
		},
		{
			name: "merchant payee",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayeeType = " MERCHANT "
				in.PayeeID = "merchant-1"
			},
		},
		{
			name: "payment_provider",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayeeType = "payment_provider"
			},
		},
		{
			name: "external",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerType = "external"
			},
		},
		{
			name: "unknown",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerType = "unknown"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validCreateCheckoutPaymentInput()
			tt.mutate(&in)
			_, err := normalizeCreateCheckoutPaymentInput(in)
			require.Error(t, err)
			require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
		})
	}
}

func TestCanonicalizePartyType_DoesNotRewritePartyIDSemantics(t *testing.T) {
	t.Parallel()

	require.Equal(t, "customer", canonicalizePartyType(" CUSTOMER "))
	require.Equal(t, "platform", canonicalizePartyType("Platform"))
	require.Equal(t, "", canonicalizePartyType("   "))
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
