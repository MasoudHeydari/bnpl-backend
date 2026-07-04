package checkoutsessionuc

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

func TestUpsertReturnDestinationInput_ValidateDestinationName(t *testing.T) {
	t.Parallel()

	merchantID := model.MerchantID(uuid.New())
	policy := returndestination.Development()

	valid := UpsertReturnDestinationInput{
		MerchantID:      merchantID,
		DestinationName: "damz-web",
		ReturnURL:       "https://merchant.example/return",
	}
	require.NoError(t, valid.validate(policy))

	for _, name := range []string{
		"",
		"Damz",
		"damz_web",
		"damz web",
		"damz--web",
		"-damz",
		"damz-",
		"foo/bar",
		"../foo",
		"سلام",
		strings.Repeat("a", 65),
	} {
		err := (UpsertReturnDestinationInput{
			MerchantID:      merchantID,
			DestinationName: name,
			ReturnURL:       "https://merchant.example/return",
		}).validate(policy)
		require.Error(t, err, "name=%q", name)
	}
}

func TestUpsertReturnDestinationInput_ValidateReturnURLPolicy(t *testing.T) {
	t.Parallel()

	merchantID := model.MerchantID(uuid.New())

	loopback := UpsertReturnDestinationInput{
		MerchantID:      merchantID,
		DestinationName: "default",
		ReturnURL:       "http://localhost:3000/bnpl/return",
	}
	require.NoError(t, loopback.validate(returndestination.Development()))
	requireCerr(
		t,
		loopback.validate(returndestination.Strict()),
		cerr.CodeInvalidArgument,
		"error.return_url_must_use_https",
	)

	publicHTTP := UpsertReturnDestinationInput{
		MerchantID:      merchantID,
		DestinationName: "default",
		ReturnURL:       "http://merchant.example/return",
	}
	requireCerr(
		t,
		publicHTTP.validate(returndestination.Development()),
		cerr.CodeInvalidArgument,
		"error.return_url_must_use_https",
	)

	for _, raw := range []string{
		"javascript:alert(1)",
		"https://u:p@example.com/return",
		"https://example.com/r?a=1",
		"https://example.com/r?",
		"https://example.com/r#foo",
		"https://example.com/r#",
		" https://example.com/return",
		"https://example.com/return ",
		"\thttps://example.com/return",
		"https://example.com/return\n",
	} {
		err := (UpsertReturnDestinationInput{
			MerchantID:      merchantID,
			DestinationName: "default",
			ReturnURL:       raw,
		}).validate(returndestination.Development())
		requireCerr(t, err, cerr.CodeInvalidArgument, "error.return_url_invalid")
	}
}

func TestCheckoutSessionRequest_ValidateReturnDestinationGrammar(t *testing.T) {
	t.Parallel()

	base := CheckoutSessionRequest{
		MerchantID:           model.MerchantID(uuid.New()),
		OnlineStoreID:        model.OnlineStoreID(uuid.New()),
		MerchantOrderID:      "order-1",
		ReturnDestination:    "default",
		RequestedAmountMinor: 1,
		Currency:             model.IRRCurrency,
	}
	require.NoError(t, base.validate())

	invalid := base
	invalid.ReturnDestination = "Foo/Bar"
	requireCerr(t, invalid.validate(), cerr.CodeInvalidArgument, "error.return_destination_invalid")
}
