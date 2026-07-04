package checkouturl_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/checkouturl"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutorigin "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutorigin"
)

func TestBuilder_Checkout(t *testing.T) {
	t.Parallel()

	origin, err := checkoutorigin.Strict([]string{"checkout.bnpl.example.com"}).Parse(
		"https://checkout.bnpl.example.com",
	)
	require.NoError(t, err)

	token := mustToken(t, "1")

	require.Equal(
		t,
		"https://checkout.bnpl.example.com/checkout/"+token.String(),
		checkouturl.New(origin).Checkout(token),
	)
}

func TestBuilder_NoDuplicateSlash(t *testing.T) {
	t.Parallel()

	origin, err := checkoutorigin.Development([]string{"bnpl.test"}).Parse("https://bnpl.test")
	require.NoError(t, err)

	token := mustToken(t, "a")

	got := checkouturl.New(origin).Checkout(token)
	require.Equal(t, "https://bnpl.test/checkout/"+token.String(), got)
	require.NotContains(t, got, "//checkout")
}

func TestBuilder_ZeroInputsReturnEmpty(t *testing.T) {
	t.Parallel()

	token := mustToken(t, "1")

	require.Empty(t, checkouturl.Builder{}.Checkout(token))
	require.Empty(t, checkouturl.New(checkoutorigin.PublicOrigin{}).Checkout(token))
	require.Empty(t, checkouturl.New(mustOrigin(t)).Checkout(model.CheckoutSessionToken("")))
}

func mustToken(t *testing.T, hexDigit string) model.CheckoutSessionToken {
	t.Helper()

	token, err := model.ParseCheckoutSessionToken(model.CheckoutSessionTokenPrefix + strings.Repeat(hexDigit, 64))
	require.NoError(t, err)

	return token
}

func mustOrigin(t *testing.T) checkoutorigin.PublicOrigin {
	t.Helper()

	origin, err := checkoutorigin.Development([]string{"bnpl.test"}).Parse("https://bnpl.test")
	require.NoError(t, err)

	return origin
}
