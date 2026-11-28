package creditallocationuc

import (
	"context"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	v3 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v3"
)

func TestAllocationRuleGuardrails(t *testing.T) {
	uc := &UseCase{installments: v1.New(), monthly: v3.New()}
	group := "bank_employees"
	for _, tc := range []struct {
		name  string
		input RuleInput
		valid bool
	}{
		{"disabled", RuleInput{Scope: model.DefaultCreditAllocationScope, Currency: model.IRRCurrency, CreditKind: model.PayInInstallmentsCreditKind, Strategy: model.DisabledCreditAllocationStrategy}, true},
		{"default fixed prohibited", RuleInput{Scope: model.DefaultCreditAllocationScope, Currency: model.IRRCurrency, CreditKind: model.PayNextMonthCreditKind, Strategy: model.FixedLimitCreditAllocationStrategy, FixedAmountMinor: 500000000}, false},
		{"zero fixed", RuleInput{Scope: model.GroupCreditAllocationScope, GroupCode: &group, Currency: model.IRRCurrency, CreditKind: model.PayInInstallmentsCreditKind, Strategy: model.FixedLimitCreditAllocationStrategy}, false},
		{"over ceiling", RuleInput{Scope: model.GroupCreditAllocationScope, GroupCode: &group, Currency: model.IRRCurrency, CreditKind: model.PayInInstallmentsCreditKind, Strategy: model.FixedLimitCreditAllocationStrategy, FixedAmountMinor: 2000000001}, false},
		{"risk not implemented", RuleInput{Scope: model.DefaultCreditAllocationScope, Currency: model.IRRCurrency, CreditKind: model.PayInInstallmentsCreditKind, Strategy: model.RiskBasedCreditAllocationStrategy}, false},
		{"default cannot inherit", RuleInput{Scope: model.DefaultCreditAllocationScope, Currency: model.IRRCurrency, CreditKind: model.PayInInstallmentsCreditKind, Strategy: model.InheritCreditAllocationStrategy}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := uc.validateRule(tc.input) == nil; got != tc.valid {
				t.Fatalf("valid = %v; want %v", got, tc.valid)
			}
		})
	}
}

func TestCurrencyAndPublicRuleGuards(t *testing.T) {
	uc := &UseCase{installments: v1.New(), monthly: v3.New()}
	group := "bank_employees"
	input := RuleInput{Scope: model.GroupCreditAllocationScope, GroupCode: &group, CreditKind: model.PayInInstallmentsCreditKind, Currency: model.IRRCurrency, Strategy: model.FixedLimitCreditAllocationStrategy, FixedAmountMinor: 500000000}
	if err := uc.validateRule(input); err != nil {
		t.Fatal(err)
	}
	for _, currency := range []model.Currency{"", model.USDCurrency, model.EURCurrency, "irr", "IRT"} {
		input.Currency = currency
		if uc.validateRule(input) == nil {
			t.Fatalf("accepted currency %q", currency)
		}
	}
	enabled := true
	if _, err := uc.PutDefault(context.Background(), DefaultInput{Currency: model.IRRCurrency, CreditKind: model.PayInInstallmentsCreditKind, Enabled: &enabled}, "admin"); err == nil {
		t.Fatal("enabled unavailable assessor")
	}
	if _, err := uc.PutRule(context.Background(), RuleInput{Scope: model.DefaultCreditAllocationScope, Currency: model.IRRCurrency, CreditKind: model.PayInInstallmentsCreditKind, Strategy: model.FixedLimitCreditAllocationStrategy, FixedAmountMinor: 1}, "admin"); err == nil {
		t.Fatal("accepted fixed public credit")
	}
}
