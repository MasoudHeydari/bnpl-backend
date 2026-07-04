package v1

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

const (
	policyID      = "checkout_contract_approval"
	policyVersion = "v1"

	installmentCount       = 4
	decisionTTL            = 15 * time.Minute
	gracePeriodDays        = uint(5)
	maxFinancedAmountMinor = uint64(2_000_000_000)

	// firstInstallmentPaymentWindow is how long the customer has to pay the
	// first installment after it is offered. It is aligned with decisionTTL
	// by value, though the two remain independent, explicitly separate terms.
	firstInstallmentPaymentWindow = 15 * time.Minute
)

type policyV1 struct{}

// New constructs the v1 checkout policy. The returned value is immutable and
// entirely hard-coded; there is no config/DB/rule-engine input.
func New() checkout.Policy {
	return policyV1{}
}

// Ref returns the v1 checkout policy identity.
func (policyV1) Ref() policy.Ref {
	return policy.Ref{ID: policyID, Version: policyVersion}
}

// Terms returns the hard-coded v1 checkout business terms.
func (policyV1) Terms() checkout.Terms {
	return checkout.Terms{
		InstallmentCount:               installmentCount,
		DecisionTTL:                    decisionTTL,
		MaxFinancedAmountMinor:         maxFinancedAmountMinor,
		FinancingMode:                  checkout.PartialWithUpfrontFinancing,
		ActivationMode:                 checkout.ActivationAfterInitialPayment,
		ScheduleAnchor:                 checkout.ScheduleFromDecision,
		FirstInstallmentPaymentWindow:  firstInstallmentPaymentWindow,
		FirstInstallmentDueAfterMonths: 0,
		GracePeriodDays:                gracePeriodDays,
	}
}
