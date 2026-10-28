package contractuc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCheckoutActivationCollectionAmount_IsUpfrontPlusFirstPrincipal(t *testing.T) {
	t.Parallel()

	attempt := model.PaymentAttempt{
		ID: model.PaymentAttemptID(uuid.New()),
		Items: []model.PaymentAttemptItem{
			{
				ItemType:    model.UpfrontPaymentAttemptItemType,
				Effect:      model.DebitPaymentAttemptItemEffect,
				AmountMinor: 20,
			},
			{
				ItemType:    model.PrincipalPaymentAttemptItemType,
				Effect:      model.DebitPaymentAttemptItemEffect,
				AmountMinor: 20,
			},
		},
	}

	total, err := checkoutActivationCollectionAmount(attempt)
	require.NoError(t, err)
	require.Equal(t, uint64(40), total)
}

func TestCheckoutActivationCollectionAmount_IncludesCharges(t *testing.T) {
	t.Parallel()

	attempt := model.PaymentAttempt{
		ID: model.PaymentAttemptID(uuid.New()),
		Items: []model.PaymentAttemptItem{
			{
				ItemType:    model.PrincipalPaymentAttemptItemType,
				Effect:      model.DebitPaymentAttemptItemEffect,
				AmountMinor: 20,
			},
			{
				ItemType:    model.ChargePaymentAttemptItemType,
				Effect:      model.DebitPaymentAttemptItemEffect,
				AmountMinor: 2,
			},
		},
	}

	amount, err := checkoutActivationCollectionAmount(attempt)
	require.NoError(t, err)
	require.EqualValues(t, 22, amount)
}
