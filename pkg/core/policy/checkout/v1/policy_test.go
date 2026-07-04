package v1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
)

func TestPolicyV1_StableIdentityAndTerms(t *testing.T) {
	t.Parallel()

	p := v1.New()

	require.Equal(t, policy.Ref{ID: "checkout_contract_approval", Version: "v1"}, p.Ref())
	require.Equal(t, checkout.Terms{
		InstallmentCount:              4,
		DecisionTTL:                   15 * time.Minute,
		MaxFinancedAmountMinor:        2_000_000_000,
		FinancingMode:                 checkout.PartialWithUpfrontFinancing,
		ActivationMode:                checkout.ActivationAfterInitialPayment,
		ScheduleAnchor:                checkout.ScheduleFromContractActivation,
		FirstInstallmentPaymentWindow: 15 * time.Minute,
		Repayment: checkout.RepaymentTerms{
			Cadence: checkout.RepaymentCadence{
				Unit:  checkout.CadenceJalaliMonth,
				Every: 1,
			},
			GracePeriodDays: 5,
			Reminder: checkout.ReminderTerms{
				LeadDays: 3,
			},
		},
	}, p.Terms())
}

func TestPolicyV1_TermsDeterministicForRepeatedCalls(t *testing.T) {
	t.Parallel()

	p := v1.New()

	require.Equal(t, p.Terms(), v1.New().Terms())
}
