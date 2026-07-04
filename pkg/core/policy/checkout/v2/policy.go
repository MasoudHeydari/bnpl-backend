package v2

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

const (
	policyID      = "checkout_contract_approval"
	policyVersion = "v2"

	installmentCount       = 3
	decisionTTL            = 15 * time.Minute
	gracePeriodDays        = uint(5)
	maxFinancedAmountMinor = uint64(2_000_000_000)
	firstDueAfterMonths    = 1
)

type policyV2 struct{}

// New constructs the immutable v2 checkout policy.
func New() checkout.Policy {
	return policyV2{}
}

// Ref returns the v2 checkout policy identity.
func (policyV2) Ref() policy.Ref {
	return policy.Ref{ID: policyID, Version: policyVersion}
}

// Terms returns the hard-coded v2 checkout business terms.
func (policyV2) Terms() checkout.Terms {
	return checkout.Terms{
		InstallmentCount:               installmentCount,
		DecisionTTL:                    decisionTTL,
		MaxFinancedAmountMinor:         maxFinancedAmountMinor,
		FinancingMode:                  checkout.FullFinancingOnly,
		ActivationMode:                 checkout.ActivationAfterMerchantFunding,
		ScheduleAnchor:                 checkout.ScheduleFromContractActivation,
		FirstInstallmentPaymentWindow:  0,
		FirstInstallmentDueAfterMonths: firstDueAfterMonths,
		GracePeriodDays:                gracePeriodDays,
	}
}
