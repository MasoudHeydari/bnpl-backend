package decisionuc

import (
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestValidateCreateInput_RejectsAmountOutsideSignedBigintRange(t *testing.T) {
	t.Parallel()

	input := CreateInput{
		CheckoutKey:       "checkout-key",
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
		PurchaseContext: PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: model.PosGpsID(uuid.New()),
		},
		RequestedAmountMinor: uint64(math.MaxInt64) + 1,
		Currency:             model.Currency("IRR"),
	}

	require.Error(t, validateCreateInput(input))
}

func TestValidateCreateInput_RejectsMissingPOSSource(t *testing.T) {
	t.Parallel()

	input := CreateInput{
		CheckoutKey:       "checkout-key",
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
		PurchaseContext: PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: model.PosGpsID{},
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.Currency("IRR"),
	}

	err := validateCreateInput(input)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}
