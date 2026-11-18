package businessrule

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

const (
	// FullFinancingOnlyRuleID is the stable id of the launch business rule.
	FullFinancingOnlyRuleID = "checkout_full_financing_only"

	// FullFinancingOnlyRuleVersion is the version of the launch business rule.
	FullFinancingOnlyRuleVersion = "v1"
)

// ShortfallAction describes what the rule does when the customer's available
// credit cannot cover the full purchase amount.
type ShortfallAction string

const (
	// RejectShortfall rejects the purchase; the customer is never asked for an
	// upfront cash top-up.
	RejectShortfall ShortfallAction = "reject"
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
}

// Ref returns the rule identity (id and version).
func (r Rule) Ref() policy.Ref { return policy.Ref{ID: r.id, Version: r.version} }

// ID returns the stable rule id.
func (r Rule) ID() string { return r.id }

// Version returns the rule version.
func (r Rule) Version() string { return r.version }

// ValidatePolicyTerms reports whether the checkout policy terms comply with
// the rule. It is intended for bootstrap-time fail-fast checks.
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

// fullFinancingOnlyV1 is the launch rule: finance the full amount or reject.
var fullFinancingOnlyV1 = Rule{
	id:            FullFinancingOnlyRuleID,
	version:       FullFinancingOnlyRuleVersion,
	FinancingMode: checkout.FullFinancingOnly,
	Shortfall:     RejectShortfall,
}

// Active returns the Checkout Business Rule applied to new checkout decisions.
func Active() Rule { return fullFinancingOnlyV1 }
