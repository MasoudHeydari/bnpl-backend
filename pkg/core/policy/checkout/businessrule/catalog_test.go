package businessrule_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout/businessrule"
	v2 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v2"
	v3 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v3"
)

func TestActive_LaunchRule(t *testing.T) {
	t.Parallel()

	rule := businessrule.Active()
	require.Equal(t, "checkout_full_financing_only", rule.ID())
	require.Equal(t, "v1", rule.Version())
	require.Equal(t, rule.ID(), rule.Ref().ID)
	require.Equal(t, rule.Version(), rule.Ref().Version)
	require.Equal(t, checkout.FullFinancingOnly, rule.FinancingMode)
	require.Equal(t, businessrule.RejectShortfall, rule.Shortfall)
}

func TestValidatePolicyTerms(t *testing.T) {
	t.Parallel()

	rule := businessrule.Active()
	require.NoError(t, rule.ValidatePolicyTerms(v2.New().Terms()))
	require.NoError(t, rule.ValidatePolicyTerms(v3.New().Terms()))

	partial := v2.New().Terms()
	partial.FinancingMode = checkout.PartialWithUpfrontFinancing
	require.Error(t, rule.ValidatePolicyTerms(partial))
}
