package policy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	initialcreditcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/catalog"
)

func TestTrustedCustomerBundle_MatchesFamilyTrustedRefs(t *testing.T) {
	t.Parallel()

	bundle := policy.TrustedCustomerBundle()
	require.Len(t, bundle, 2)
	require.Equal(t, model.CheckoutPolicyFamily, bundle[0].Family)
	require.Equal(t, checkoutcatalog.TrustedRef(), bundle[0].Ref)
	require.Equal(t, model.InitialCreditLimitPolicyFamily, bundle[1].Family)
	require.Equal(t, initialcreditcatalog.TrustedRef(), bundle[1].Ref)
}
