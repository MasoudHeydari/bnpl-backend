package v3

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
)

// Monthly limits are governed separately from installment credit. The initial
// 30M IRR is the independently governed initial monthly revolving capacity.
// It never inherits the 100M IRR pay-in-four baseline. Future changes must
// use another immutable policy version.
const (
	policyID = "customer_monthly_initial_limit"
	policyVersion = "v1"
	defaultAmountMinor = uint64(30_000_000)
	maxAmountMinor = uint64(2_000_000_000)
	validFor = 365 * 24 * time.Hour
	reasonCode = "MONTHLY_INITIAL_CREDIT_LIMIT_POLICY_APPROVED"
)
type policyV1 struct{}
func New() initialcreditlimit.Policy { return policyV1{} }
func (policyV1) Ref() policy.Ref {
	return policy.Ref{ID:policyID,Version:policyVersion}
}
func (policyV1) Terms() initialcreditlimit.Terms {
	return initialcreditlimit.Terms{
		DefaultAmountMinor: defaultAmountMinor,MaxAmountMinor:maxAmountMinor,
		Currency:model.IRRCurrency,ValidFor:validFor,ApprovalReasonCode:reasonCode,
	}
}
