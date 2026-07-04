package initialcreditlimit

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
)

// Policy defines one version of the initial customer credit-limit policy
// family. All versions expose the same shape; business differences live in
// immutable Terms values rather than in application/config constants.
type Policy interface {
	// Ref returns this policy version's stable identity.
	Ref() policy.Ref
	// Terms returns the explicit hard-coded business terms for this version.
	Terms() Terms
}

// Terms contains the code-owned initial-credit business limits for one policy version.
type Terms struct {
	// DefaultAmountMinor is the amount assigned by the current default assessor
	// path when the customer is approved.
	DefaultAmountMinor uint64
	// MaxAmountMinor is the absolute policy ceiling for any assessment governed
	// by this version. No caller may substitute its own ceiling.
	MaxAmountMinor uint64
	// Currency is the currency of DefaultAmountMinor and MaxAmountMinor.
	Currency model.Currency
	// ValidFor is how long an approved initial limit remains valid.
	ValidFor time.Duration
	// ApprovalReasonCode is the stable reason code for a policy-approved limit.
	ApprovalReasonCode string
}
