package businessrule

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

const (
	// FullFinancingOnlyRuleID is the stable id of the launch reject rule.
	// Alias identity: reject / full-financing-only.
	FullFinancingOnlyRuleID = "checkout_full_financing_only"

	// FullFinancingOnlyRuleVersion is the version of the launch reject rule.
	FullFinancingOnlyRuleVersion = "v1"

	// RejectRuleID is the catalog name for zero-upfront reject-on-shortfall.
	RejectRuleID = FullFinancingOnlyRuleID

	// RejectRuleVersion is the launch reject rule version.
	RejectRuleVersion = FullFinancingOnlyRuleVersion

	// PayDifferenceUpfrontRuleID identifies the inactive upfront shortfall rule.
	PayDifferenceUpfrontRuleID = "checkout_pay_difference_upfront"

	// PayDifferenceUpfrontRuleVersion is the catalog version for that rule.
	PayDifferenceUpfrontRuleVersion = "v1"

	// FinanceExcessRuleID identifies the inactive overdraft financing rule.
	FinanceExcessRuleID = "checkout_finance_excess"

	// FinanceExcessRuleVersion is the catalog version for that rule.
	FinanceExcessRuleVersion = "v1"
)

// ShortfallAction describes what the rule does when the customer's available
// credit cannot cover the full purchase amount.
type ShortfallAction string

const (
	// RejectShortfall rejects the purchase; the customer is never asked for an
	// upfront cash top-up.
	RejectShortfall ShortfallAction = "reject"

	// PayDifferenceUpfrontShortfall would require an authoritative customer
	// cash payment at checkout for the shortfall. Inactive until FC matching
	// constraints and checkout-time customer payments are supported.
	PayDifferenceUpfrontShortfall ShortfallAction = "pay_difference_upfront"

	// FinanceExcessShortfall would finance above available credit within
	// explicit risk bounds. Inactive; must never grant uncontrolled overdraft.
	FinanceExcessShortfall ShortfallAction = "finance_excess"
)

// Rule is an immutable compiled Checkout Business Rule.
type Rule struct {
	id      string
	version string

	// FinancingMode is the financing mode every checkout policy must use while
	// this rule is active.
	FinancingMode checkout.FinancingMode

	// Shortfall is the action taken when available credit is below the
	// requested amount.
	Shortfall ShortfallAction

	// ActiveAtBuild reports whether this rule may govern NEW decisions in
	// this binary. Historical Resolve still returns inactive rules.
	ActiveAtBuild bool
}

// Ref returns the rule identity (id and version).
func (r Rule) Ref() policy.Ref { return policy.Ref{ID: r.id, Version: r.version} }

// ID returns the stable rule id.
func (r Rule) ID() string { return r.id }

// Version returns the rule version.
func (r Rule) Version() string { return r.version }

// ValidatePolicyTerms reports whether the checkout policy terms comply with
// the rule. It is intended for bootstrap-time fail-fast checks and for
// revalidation of a persisted Decision against its original rule.
func (r Rule) ValidatePolicyTerms(terms checkout.Terms) error {
	const op = "businessrule.Rule.ValidatePolicyTerms"

	if terms.FinancingMode != r.FinancingMode {
		return fmt.Errorf(
			"%s: rule %s requires financing mode %q, got %q",
			op,
			r.Ref(),
			r.FinancingMode,
			terms.FinancingMode,
		)
	}

	return nil
}

// rejectV1 is the launch rule: finance the full amount or reject (zero upfront).
var rejectV1 = Rule{
	id:            RejectRuleID,
	version:       RejectRuleVersion,
	FinancingMode: checkout.FullFinancingOnly,
	Shortfall:     RejectShortfall,
	ActiveAtBuild: true,
}

// payDifferenceUpfrontV1 remains catalogued but inactive. Enabling it requires
// checkout-time customer payments and authoritative FC matching constraints.
var payDifferenceUpfrontV1 = Rule{
	id:            PayDifferenceUpfrontRuleID,
	version:       PayDifferenceUpfrontRuleVersion,
	FinancingMode: checkout.PartialWithUpfrontFinancing,
	Shortfall:     PayDifferenceUpfrontShortfall,
	ActiveAtBuild: false,
}

// financeExcessV1 remains catalogued but inactive. Enabling it requires
// explicit authorization and financial/risk bounds; it must never become a
// silent permissive default.
var financeExcessV1 = Rule{
	id:            FinanceExcessRuleID,
	version:       FinanceExcessRuleVersion,
	FinancingMode: checkout.FullFinancingOnly,
	Shortfall:     FinanceExcessShortfall,
	ActiveAtBuild: false,
}

var catalog = map[string]Rule{
	key(rejectV1.ID(), rejectV1.Version()):                             rejectV1,
	key(payDifferenceUpfrontV1.ID(), payDifferenceUpfrontV1.Version()): payDifferenceUpfrontV1,
	key(financeExcessV1.ID(), financeExcessV1.Version()):               financeExcessV1,
}

func key(id, version string) string { return id + ":" + version }

// Active returns the Checkout Business Rule applied to NEW checkout decisions.
// Launch builds select reject / full-financing-only only.
func Active() Rule { return rejectV1 }

// Resolve returns a historical or active rule by exact id and version.
// Unknown references fail closed; callers must never fall back to Active().
func Resolve(id, version string) (Rule, error) {
	const op = "businessrule.Resolve"

	if id == "" || version == "" {
		return Rule{}, fmt.Errorf("%s: business rule id and version are required", op)
	}

	rule, ok := catalog[key(id, version)]
	if !ok {
		return Rule{}, fmt.Errorf("%s: unknown checkout business rule %s:%s", op, id, version)
	}

	return rule, nil
}

// Catalog returns every compiled rule for audits and tests.
func Catalog() []Rule {
	return []Rule{rejectV1, payDifferenceUpfrontV1, financeExcessV1}
}
