package checkoutsessionuc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func mustCheckoutSessionID(t *testing.T, value string) model.CheckoutSessionID {
	t.Helper()

	id, err := model.ParseCheckoutSessionID(value)
	require.NoError(t, err)

	return id
}

func TestMerchantReturnURL_AppendsCheckoutIdentityAndState(t *testing.T) {
	t.Parallel()

	token := model.CheckoutSessionToken(
		"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	session := model.CheckoutSession{
		ID:            mustCheckoutSessionID(t, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
		PublicToken:   token,
		ReturnURL:     "https://shop.example/bnpl/return?source=bnpl",
		MerchantState: "cart-42",
	}

	returnURL, err := merchantReturnURL(session)
	require.NoError(t, err)
	require.Equal(
		t,
		"https://shop.example/bnpl/return?"+
			"checkout_session_id="+token.String()+"&source=bnpl&state=cart-42",
		returnURL,
	)
}

func TestMerchantReturnURL_OmitsEmptyMerchantState(t *testing.T) {
	t.Parallel()

	token := model.CheckoutSessionToken(
		"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	session := model.CheckoutSession{
		ID:          mustCheckoutSessionID(t, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
		PublicToken: token,
		ReturnURL:   "https://shop.example/bnpl/return",
	}

	returnURL, err := merchantReturnURL(session)
	require.NoError(t, err)
	require.Equal(
		t,
		"https://shop.example/bnpl/return?checkout_session_id="+token.String(),
		returnURL,
	)
}
