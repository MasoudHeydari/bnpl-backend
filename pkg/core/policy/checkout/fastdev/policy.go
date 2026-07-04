//go:build development

package fastdev

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

const (
	policyID      = "checkout_contract_approval"
	policyVersion = "v1_fast_5d"

	installmentCount       = 4
	decisionTTL            = 15 * time.Minute
	gracePeriodDays        = uint(3)
	maxFinancedAmountMinor = uint64(2_000_000_000)
	reminderLeadDays       = uint(3)
	cadenceEveryDays       = uint(5)

	firstInstallmentPaymentWindow = 15 * time.Minute
)

// policyFastDev is a Development-only checkout policy that preserves v1
// economics (partial financing + initial payment) while using a five calendar
// day repayment cadence so reminder, grace, default, and recovery flows can be
// exercised without waiting Jalali months.
type policyFastDev struct{}

// New constructs the Development fast-cadence checkout policy.
func New() checkout.Policy {
	return policyFastDev{}
}

// Ref returns the fast-dev checkout policy identity.
func (policyFastDev) Ref() policy.Ref {
	return policy.Ref{ID: policyID, Version: policyVersion}
}

// Terms returns the hard-coded Development fast-cadence terms.
func (policyFastDev) Terms() checkout.Terms {
	return checkout.Terms{
		InstallmentCount:              installmentCount,
		DecisionTTL:                   decisionTTL,
		MaxFinancedAmountMinor:        maxFinancedAmountMinor,
		FinancingMode:                 checkout.PartialWithUpfrontFinancing,
		ActivationMode:                checkout.ActivationAfterInitialPayment,
		ScheduleAnchor:                checkout.ScheduleFromContractActivation,
		FirstInstallmentPaymentWindow: firstInstallmentPaymentWindow,
		Repayment: checkout.RepaymentTerms{
			Cadence: checkout.RepaymentCadence{
				Unit:  checkout.CadenceCalendarDay,
				Every: cadenceEveryDays,
			},
			GracePeriodDays: gracePeriodDays,
			Reminder: checkout.ReminderTerms{
				LeadDays: reminderLeadDays,
			},
		},
	}
}
