package checkout_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
	checkoutv2 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v2"
)

func validMonthlyRepayment() checkout.RepaymentTerms {
	return checkout.RepaymentTerms{
		Cadence: checkout.RepaymentCadence{
			Unit:  checkout.CadenceJalaliMonth,
			Every: 1,
		},
		GracePeriodDays: 5,
		Reminder: checkout.ReminderTerms{
			LeadDays: 3,
		},
	}
}

func TestValidateTerms_RejectsInvalidCadenceAndGrace(t *testing.T) {
	t.Parallel()

	base := checkout.Terms{
		InstallmentCount:              4,
		DecisionTTL:                   15 * time.Minute,
		MaxFinancedAmountMinor:        1_000_000,
		FinancingMode:                 checkout.PartialWithUpfrontFinancing,
		ActivationMode:                checkout.ActivationAfterInitialPayment,
		ScheduleAnchor:                checkout.ScheduleFromContractActivation,
		FirstInstallmentPaymentWindow: 15 * time.Minute,
		Repayment:                     validMonthlyRepayment(),
	}

	tests := []struct {
		name   string
		mutate func(terms *checkout.Terms)
	}{
		{
			name: "zero day cadence",
			mutate: func(terms *checkout.Terms) {
				terms.Repayment.Cadence = checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 0}
			},
		},
		{
			name: "three day cadence",
			mutate: func(terms *checkout.Terms) {
				terms.Repayment.Cadence = checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 3}
			},
		},
		{
			name: "unknown cadence unit",
			mutate: func(terms *checkout.Terms) {
				terms.Repayment.Cadence = checkout.RepaymentCadence{Unit: "lunar", Every: 4}
			},
		},
		{
			name: "four day cadence with three day grace",
			mutate: func(terms *checkout.Terms) {
				terms.Repayment.Cadence = checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 4}
				terms.Repayment.GracePeriodDays = 3
			},
		},
		{
			name: "five day cadence with four day grace",
			mutate: func(terms *checkout.Terms) {
				terms.Repayment.Cadence = checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 5}
				terms.Repayment.GracePeriodDays = 4
			},
		},
		{
			name: "zero reminder lead",
			mutate: func(terms *checkout.Terms) {
				terms.Repayment.Reminder.LeadDays = 0
			},
		},
		{
			name: "lead days >= cadence spacing",
			mutate: func(terms *checkout.Terms) {
				terms.Repayment.Cadence = checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 5}
				terms.Repayment.GracePeriodDays = 3
				terms.Repayment.Reminder.LeadDays = 5
			},
		},
		{
			name: "invalid activation mode",
			mutate: func(terms *checkout.Terms) {
				terms.ActivationMode = "instant"
			},
		},
		{
			name: "invalid schedule anchor for initial payment",
			mutate: func(terms *checkout.Terms) {
				terms.ScheduleAnchor = checkout.ScheduleFromDecision
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			terms := base
			tt.mutate(&terms)
			require.Error(t, checkout.ValidateTerms(terms))
		})
	}
}

func TestValidateTerms_AcceptsRegisteredPolicies(t *testing.T) {
	t.Parallel()

	require.NoError(t, checkout.ValidateTerms(checkoutv1.New().Terms()))
	require.NoError(t, checkout.ValidateTerms(checkoutv2.New().Terms()))
}

func TestNewRegistry_RejectsInvalidRegisteredPolicy(t *testing.T) {
	t.Parallel()

	invalid := invalidCheckoutPolicy{
		ref: policy.Ref{ID: "checkout_contract_approval", Version: "invalid"},
		terms: func() checkout.Terms {
			terms := checkoutv1.New().Terms()
			terms.Repayment.Cadence.Every = 0
			return terms
		}(),
	}

	_, err := checkout.NewRegistry(checkoutv1.New(), invalid)
	require.Error(t, err)
}

type invalidCheckoutPolicy struct {
	ref   policy.Ref
	terms checkout.Terms
}

func (m invalidCheckoutPolicy) Ref() policy.Ref { return m.ref }

func (m invalidCheckoutPolicy) Terms() checkout.Terms { return m.terms }

func (invalidCheckoutPolicy) EvaluateEligibility(checkout.EligibilityInput) checkout.EligibilityResult {
	return checkout.EligibilityResult{}
}

func (invalidCheckoutPolicy) EvaluateFinancing(checkout.FinancingInput) checkout.FinancingResult {
	return checkout.FinancingResult{}
}

func (invalidCheckoutPolicy) BuildInstallmentSchedule(checkout.ScheduleInput) (checkout.ScheduleResult, error) {
	return checkout.ScheduleResult{}, nil
}

func (invalidCheckoutPolicy) BuildContractPlan(checkout.ContractPlanInput) (checkout.ContractPlan, error) {
	return checkout.ContractPlan{}, nil
}

func TestCadenceSchedules_CalendarAndJalali(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		cadence checkout.RepaymentCadence
		mode    checkout.ActivationMode
		orders  []int
		wantDue func(order int) time.Time
	}{
		{
			name:    "every 4 calendar days trusted",
			cadence: checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 4},
			mode:    checkout.ActivationAfterMerchantFunding,
			orders:  []int{1, 2, 3},
			wantDue: func(order int) time.Time {
				return checkout.AddCalendarDays(anchor, 4*order)
			},
		},
		{
			name:    "every 5 calendar days trusted",
			cadence: checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 5},
			mode:    checkout.ActivationAfterMerchantFunding,
			orders:  []int{1, 2, 3},
			wantDue: func(order int) time.Time {
				return checkout.AddCalendarDays(anchor, 5*order)
			},
		},
		{
			name:    "weekly trusted",
			cadence: checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 7},
			mode:    checkout.ActivationAfterMerchantFunding,
			orders:  []int{1, 2, 3},
			wantDue: func(order int) time.Time {
				return checkout.AddCalendarDays(anchor, 7*order)
			},
		},
		{
			name:    "biweekly trusted",
			cadence: checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 14},
			mode:    checkout.ActivationAfterMerchantFunding,
			orders:  []int{1, 2, 3},
			wantDue: func(order int) time.Time {
				return checkout.AddCalendarDays(anchor, 14*order)
			},
		},
		{
			name:    "one jalali month trusted",
			cadence: checkout.RepaymentCadence{Unit: checkout.CadenceJalaliMonth, Every: 1},
			mode:    checkout.ActivationAfterMerchantFunding,
			orders:  []int{1, 2, 3},
			wantDue: func(order int) time.Time {
				return jalali.AddMonthsClamped(anchor, order)
			},
		},
		{
			name:    "two jalali months trusted",
			cadence: checkout.RepaymentCadence{Unit: checkout.CadenceJalaliMonth, Every: 2},
			mode:    checkout.ActivationAfterMerchantFunding,
			orders:  []int{1, 2, 3},
			wantDue: func(order int) time.Time {
				return jalali.AddMonthsClamped(anchor, 2*order)
			},
		},
		{
			name:    "every 5 days normal recurring from activation",
			cadence: checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 5},
			mode:    checkout.ActivationAfterInitialPayment,
			orders:  []int{2, 3, 4},
			wantDue: func(order int) time.Time {
				return checkout.AddCalendarDays(anchor, 5*(order-1))
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			terms := checkout.Terms{
				ActivationMode:                tt.mode,
				FirstInstallmentPaymentWindow: 15 * time.Minute,
				Repayment: checkout.RepaymentTerms{
					Cadence: tt.cadence,
				},
			}

			var prev time.Time
			for _, order := range tt.orders {
				due, err := checkout.DueAtForInstallmentOrder(terms, anchor, order)
				require.NoError(t, err)
				require.Equal(t, tt.wantDue(order), due)
				if !prev.IsZero() {
					require.True(t, due.After(prev))
				}
				prev = due
			}
		})
	}
}

func TestAddCalendarDays_PreservesWallClockAcrossDST(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	start := time.Date(2026, time.March, 20, 15, 30, 0, 0, loc)
	next := checkout.AddCalendarDays(start, 5)
	require.Equal(t, 15, next.Hour())
	require.Equal(t, 30, next.Minute())
	require.Equal(t, time.Date(2026, time.March, 25, 15, 30, 0, 0, loc), next)
}

func TestValidateGeneratedSchedule_ExactCadenceAndGrace(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	terms := checkout.Terms{
		InstallmentCount:       3,
		DecisionTTL:            15 * time.Minute,
		MaxFinancedAmountMinor: 300,
		FinancingMode:          checkout.FullFinancingOnly,
		ActivationMode:         checkout.ActivationAfterMerchantFunding,
		ScheduleAnchor:         checkout.ScheduleFromContractActivation,
		Repayment: checkout.RepaymentTerms{
			Cadence:         checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 5},
			GracePeriodDays: 2,
			Reminder: checkout.ReminderTerms{
				LeadDays: 3,
			},
		},
	}

	drafts := make([]checkout.InstallmentDraft, 0, 3)
	for order := 1; order <= 3; order++ {
		due, err := checkout.DueAtForInstallmentOrder(terms, anchor, order)
		require.NoError(t, err)
		drafts = append(drafts, checkout.InstallmentDraft{
			Order:                order,
			PrincipalAmountMinor: 100,
			TotalAmountMinor:     100,
			DueAt:                due,
		})
	}

	require.NoError(t, checkout.ValidateGeneratedSchedule(
		terms,
		checkout.ScheduleResult{Anchor: anchor, Installments: drafts},
		300,
		0,
	))

	// Mutating a recurring due date must fail exact cadence validation.
	// Clone so the original drafts stay valid for the grace-period case.
	bad := append([]checkout.InstallmentDraft(nil), drafts...)
	bad[1].DueAt = bad[1].DueAt.Add(24 * time.Hour)
	require.Error(t, checkout.ValidateGeneratedSchedule(
		terms,
		checkout.ScheduleResult{Anchor: anchor, Installments: bad},
		300,
		0,
	))
	require.NoError(t, checkout.ValidateGeneratedSchedule(
		terms,
		checkout.ScheduleResult{Anchor: anchor, Installments: drafts},
		300,
		0,
	))

	// Grace G=4 with C=5 violates G < C-1.
	badGrace := terms
	badGrace.Repayment.GracePeriodDays = 4
	require.Error(t, checkout.ValidateGeneratedSchedule(
		badGrace,
		checkout.ScheduleResult{Anchor: anchor, Installments: drafts},
		300,
		0,
	))
}

func TestValidateGeneratedSchedule_NormalCustomerInitialDeadline(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	terms := checkout.Terms{
		InstallmentCount:              3,
		DecisionTTL:                   15 * time.Minute,
		MaxFinancedAmountMinor:        300,
		FinancingMode:                 checkout.PartialWithUpfrontFinancing,
		ActivationMode:                checkout.ActivationAfterInitialPayment,
		ScheduleAnchor:                checkout.ScheduleFromContractActivation,
		FirstInstallmentPaymentWindow: 30 * time.Minute,
		Repayment: checkout.RepaymentTerms{
			Cadence:         checkout.RepaymentCadence{Unit: checkout.CadenceCalendarDay, Every: 7},
			GracePeriodDays: 3,
			Reminder: checkout.ReminderTerms{
				LeadDays: 3,
			},
		},
	}

	drafts := make([]checkout.InstallmentDraft, 0, 3)
	for order := 1; order <= 3; order++ {
		due, err := checkout.DueAtForInstallmentOrder(terms, anchor, order)
		require.NoError(t, err)
		upfront := uint64(0)
		if order == 1 {
			upfront = 50
		}
		total := uint64(100) + upfront
		drafts = append(drafts, checkout.InstallmentDraft{
			Order:                order,
			PrincipalAmountMinor: 100,
			UpfrontAmountMinor:   upfront,
			TotalAmountMinor:     total,
			DueAt:                due,
		})
	}

	require.Equal(t, anchor.Add(30*time.Minute), drafts[0].DueAt)
	require.NoError(t, checkout.ValidateGeneratedSchedule(
		terms,
		checkout.ScheduleResult{Anchor: anchor, Installments: drafts},
		300,
		50,
	))
}
