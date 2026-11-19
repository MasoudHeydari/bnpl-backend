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

func TestResolve_HistoricalAndUnknown(t *testing.T) {
	t.Parallel()

	active := businessrule.Active()
	resolved, err := businessrule.Resolve(active.ID(), active.Version())
	require.NoError(t, err)
	require.Equal(t, active.ID(), resolved.ID())
	require.True(t, resolved.ActiveAtBuild)

	upfront, err := businessrule.Resolve(
		businessrule.PayDifferenceUpfrontRuleID,
		businessrule.PayDifferenceUpfrontRuleVersion,
	)
	require.NoError(t, err)
	require.False(t, upfront.ActiveAtBuild)
	require.Equal(t, businessrule.PayDifferenceUpfrontShortfall, upfront.Shortfall)

	excess, err := businessrule.Resolve(
		businessrule.FinanceExcessRuleID,
		businessrule.FinanceExcessRuleVersion,
	)
	require.NoError(t, err)
	require.False(t, excess.ActiveAtBuild)

	_, err = businessrule.Resolve("checkout_full_financing_only", "v999")
	require.Error(t, err)

	_, err = businessrule.Resolve("", "v1")
	require.Error(t, err)
}

func TestCatalog_ContainsInactiveRules(t *testing.T) {
	t.Parallel()

	rules := businessrule.Catalog()
	require.Len(t, rules, 3)

	var sawActive, sawUpfront, sawExcess bool
	for _, rule := range rules {
		switch rule.ID() {
		case businessrule.RejectRuleID:
			sawActive = rule.ActiveAtBuild

		case businessrule.PayDifferenceUpfrontRuleID:
			sawUpfront = !rule.ActiveAtBuild

		case businessrule.FinanceExcessRuleID:
			sawExcess = !rule.ActiveAtBuild
		}
	}

	require.True(t, sawActive && sawUpfront && sawExcess)
}
