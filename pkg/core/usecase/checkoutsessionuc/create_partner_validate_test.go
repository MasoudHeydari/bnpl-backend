package checkoutsessionuc

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCreatePartnerInput_Validate_AcceptsRawPhone(t *testing.T) {
	t.Parallel()

	for _, phone := range []string{
		"+989121234567",
		"09121234567",
		" 09121234567", // whitespace rejected later by jurisdiction canonicalizer
	} {
		t.Run(phone, func(t *testing.T) {
			t.Parallel()

			input := validCreatePartnerInput(t)
			input.CustomerPhone = phone
			require.NoError(t, input.validate())
		})
	}
}

func TestCreatePartnerInput_Validate_RejectsEmptyPhone(t *testing.T) {
	t.Parallel()

	input := validCreatePartnerInput(t)
	input.CustomerPhone = ""
	requireCerr(t, input.validate(), cerr.CodeInvalidArgument, "error.customer_phone_required")
}

func TestCreatePartnerInput_Validate_AcceptsMaxLengthPhone(t *testing.T) {
	t.Parallel()

	input := validCreatePartnerInput(t)
	input.CustomerPhone = strings.Repeat("0", model.MaxCustomerPhoneLength)
	require.NoError(t, input.validate())
}

func TestCreatePartnerInput_Validate_RejectsPhoneOverMaxLength(t *testing.T) {
	t.Parallel()

	input := validCreatePartnerInput(t)
	input.CustomerPhone = strings.Repeat("0", model.MaxCustomerPhoneLength+1)
	requireCerr(t, input.validate(), cerr.CodeInvalidArgument, "error.customer_phone_invalid")
}

func validCreatePartnerInput(t *testing.T) CreatePartnerInput {
	t.Helper()

	return CreatePartnerInput{
		CheckoutSessionRequest: CheckoutSessionRequest{
			MerchantID:           model.MerchantID(uuid.New()),
			OnlineStoreID:        model.OnlineStoreID(uuid.New()),
			MerchantOrderID:      "order-1",
			IdempotencyKey:       "attempt-1",
			ReturnDestination:    "storefront",
			RequestedAmountMinor: 150000,
			Currency:             model.IRRCurrency,
		},
		CustomerPhone: "+989121234567",
	}
}
