package v3

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

func TestMonthlyDueDateAlwaysFifthOfFollowingJalaliMonth(t *testing.T) {
	location, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		t.Fatal(err)
	}

	policy := New()
	anchors := []struct {
		name   string
		anchor jalali.Date
		next   jalali.Date
	}{
		{"early month", jalali.Date{Year: 1405, Month: 1, Day: 1}, jalali.Date{Year: 1405, Month: 2, Day: 5}},
		{"after fifth", jalali.Date{Year: 1405, Month: 1, Day: 28}, jalali.Date{Year: 1405, Month: 2, Day: 5}},
		{"year boundary", jalali.Date{Year: 1405, Month: 12, Day: 20}, jalali.Date{Year: 1406, Month: 1, Day: 5}},
	}
	for _, tc := range anchors {
		t.Run(tc.name, func(t *testing.T) {
			wall := time.Date(2026, time.March, 21, 15, 0, 0, 0, location)
			anchor := tc.anchor.ToTime(wall, location)
			due, err := checkout.DueAtForInstallmentOrder(policy.Terms(), anchor, 1)
			if err != nil {
				t.Fatal(err)
			}
			parts := jalali.FromTime(due.In(location))
			if parts != tc.next {
				t.Fatalf("due=%+v want=%+v", parts, tc.next)
			}
			if due.Hour() != 23 || due.Minute() != 59 || due.Second() != 59 {
				t.Fatalf("due time=%s want 23:59:59", due)
			}
			schedule, err := policy.BuildInstallmentSchedule(checkout.ScheduleInput{
				Now: anchor, FinancedAmountMinor: 10000, InstallmentCount: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(schedule.Installments) != 1 || !schedule.Installments[0].DueAt.Equal(due) {
				t.Fatalf("invalid monthly schedule: %+v", schedule)
			}
		})
	}
}

func TestMonthlyFinancingCannotUseAnotherAccountsCredit(t *testing.T) {
	policy := New()
	if err := checkout.ValidateTerms(policy.Terms()); err != nil {
		t.Fatal(err)
	}
	credit := model.CreditAccount{LimitAmountMinor: 1000, ExposureAmountMinor: 750, CreditKind: model.PayNextMonthCreditKind}
	financed := policy.EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 300, CreditAccount: credit,
		Eligibility: checkout.EligibilityResult{Outcome: model.ApprovedDecisionOutcome},
	})
	if financed.Outcome != model.DeniedDecisionOutcome {
		t.Fatalf("monthly financed past available limit: %+v", financed)
	}
	financed = policy.EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 200, CreditAccount: credit,
		Eligibility: checkout.EligibilityResult{Outcome: model.ApprovedDecisionOutcome},
	})
	if financed.Outcome != model.ApprovedDecisionOutcome || financed.FinancedAmountMinor != 200 || financed.UpfrontAmountMinor != 0 {
		t.Fatalf("monthly financing failed: %+v", financed)
	}
}
