package v1

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
)

const (
	policyID      = "customer_default_initial_limit"
	policyVersion = "v1"

	defaultAmountMinor = uint64(100_000_000)
	maxAmountMinor     = uint64(2_000_000_000)
	validFor           = 365 * 24 * time.Hour
	reasonCode         = "INITIAL_CREDIT_LIMIT_POLICY_APPROVED"

	currency = model.IRRCurrency
)

type policyV1 struct{}

// New constructs the v1 initial credit-limit policy. The returned value is
// immutable and entirely hard-coded; there is no config/DB/rule-engine input.
func New() initialcreditlimit.Policy { return policyV1{} }

// Ref returns the v1 initial credit-limit policy identity.
func (policyV1) Ref() policy.Ref {
	return policy.Ref{ID: policyID, Version: policyVersion}
}

// Terms returns the hard-coded v1 initial credit-limit business terms.
func (policyV1) Terms() initialcreditlimit.Terms {
	return initialcreditlimit.Terms{
		DefaultAmountMinor: defaultAmountMinor,
		MaxAmountMinor:     maxAmountMinor,
		Currency:           currency,
		ValidFor:           validFor,
		ApprovalReasonCode: reasonCode,
	}
}
