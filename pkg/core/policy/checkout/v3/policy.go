package v3

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// Version v3 introduces a distinct pay-next-month credit product without
// altering the immutable installment policies v1/v2.
const (
	policyID      = "checkout_contract_approval"
	policyVersion = "v3"

	installmentCount       = 1
	decisionTTL            = 15 * time.Minute
	// Monthly installments become overdue at Tehran midnight on the sixth.
	// Their fifth-day deadline has no additional grace days.
	gracePeriodDays        = uint(0)
	maxFinancedAmountMinor = uint64(2_000_000_000)
	reminderLeadDays       = uint(3)
)

type policyV3 struct{}

// New creates the reviewed immutable next-month checkout policy.
func New() checkout.Policy { return policyV3{} }

func (policyV3) Ref() policy.Ref { return policy.Ref{ID: policyID, Version: policyVersion} }

func (policyV3) Terms() checkout.Terms {
	return checkout.Terms{
		CreditKind: model.PayNextMonthCreditKind,
		DueDateRule: checkout.DueNextJalaliMonthFifth,
		InstallmentCount: installmentCount,
		DecisionTTL: decisionTTL,
		MaxFinancedAmountMinor: maxFinancedAmountMinor,
		FinancingMode: checkout.FullFinancingOnly,
		ActivationMode: checkout.ActivationAfterMerchantFunding,
		ScheduleAnchor: checkout.ScheduleFromContractActivation,
		FirstInstallmentPaymentWindow: 0,
		Repayment: checkout.RepaymentTerms{
			Cadence: checkout.RepaymentCadence{
				Unit: checkout.CadenceJalaliMonth,
				Every: 1,
			},
			GracePeriodDays: gracePeriodDays,
			Reminder: checkout.ReminderTerms{LeadDays: reminderLeadDays},
		},
	}
}
