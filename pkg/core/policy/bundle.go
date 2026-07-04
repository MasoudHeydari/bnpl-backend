package policy

import "github.com/parstis/bnpl-backend/pkg/core/model"

// Target identifies one policy family/ref selected by an assignment bundle.
type Target struct {
	Family model.PolicyFamily
	Ref    Ref
}

// TrustedCustomerBundle returns the policy assignments applied atomically when
// a subject is admitted to the trusted cohort (for example via CSV import).
//
// Refs are the trusted versions owned by each policy family's catalog
// (checkout/v2 and initial-credit-limit/v2). They are written here as literals
// so package policy stays a leaf dependency and does not import versioned
// policy packages (which themselves depend on policy.Ref).
func TrustedCustomerBundle() []Target {
	return []Target{
		{
			Family: model.CheckoutPolicyFamily,
			Ref:    Ref{ID: "checkout_contract_approval", Version: "v2"},
		},
		{
			Family: model.InitialCreditLimitPolicyFamily,
			Ref:    Ref{ID: "customer_default_initial_limit", Version: "v2"},
		},
	}
}
