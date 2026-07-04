package v2

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
)

const (
	policyID      = "customer_default_initial_limit"
	policyVersion = "v2"

	defaultAmountMinor = uint64(2_000_000_000)
	maxAmountMinor     = uint64(2_000_000_000)
	validFor           = 365 * 24 * time.Hour
	reasonCode         = "TRUSTED_CUSTOMER_INITIAL_CREDIT_LIMIT_POLICY_APPROVED"

	currency = model.IRRCurrency
)

type policyV2 struct{}

// New constructs the immutable trusted-customer v2 credit-limit policy.
func New() initialcreditlimit.Policy { return policyV2{} }

// Ref returns the v2 policy identity.
func (policyV2) Ref() policy.Ref {
	return policy.Ref{ID: policyID, Version: policyVersion}
}

// Terms returns the hard-coded v2 initial credit-limit business terms.
func (policyV2) Terms() initialcreditlimit.Terms {
	return initialcreditlimit.Terms{
		DefaultAmountMinor: defaultAmountMinor,
		MaxAmountMinor:     maxAmountMinor,
		Currency:           currency,
		ValidFor:           validFor,
		ApprovalReasonCode: reasonCode,
	}
}
