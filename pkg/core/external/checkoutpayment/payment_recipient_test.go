package checkoutpayment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckoutPayeeFromPrepare(t *testing.T) {
	t.Parallel()

	t.Run("platform without id", func(t *testing.T) {
		t.Parallel()

		payeeType, payeeID, err := CheckoutPayeeFromPrepare(PrepareCollectionObligationResult{
			PaymentRecipientType: " Platform ",
		})
		require.NoError(t, err)
		require.Equal(t, PartyPlatform, payeeType)
		require.Empty(t, payeeID)
	})

	t.Run("merchant with id", func(t *testing.T) {
		t.Parallel()

		payeeType, payeeID, err := CheckoutPayeeFromPrepare(PrepareCollectionObligationResult{
			PaymentRecipientType: PartyMerchant,
			PaymentRecipientID:   "merchant-1",
		})
		require.NoError(t, err)
		require.Equal(t, PartyMerchant, payeeType)
		require.Equal(t, "merchant-1", payeeID)
	})

	t.Run("missing type is not defaulted to platform", func(t *testing.T) {
		t.Parallel()

		_, _, err := CheckoutPayeeFromPrepare(PrepareCollectionObligationResult{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "payment_recipient_type is required")
	})

	t.Run("platform with id rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := CheckoutPayeeFromPrepare(PrepareCollectionObligationResult{
			PaymentRecipientType: PartyPlatform,
			PaymentRecipientID:   "merchant-1",
		})
		require.Error(t, err)
	})

	t.Run("merchant without id rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := CheckoutPayeeFromPrepare(PrepareCollectionObligationResult{
			PaymentRecipientType: PartyMerchant,
		})
		require.Error(t, err)
	})
}
