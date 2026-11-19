package model

import "testing"

func TestCreditKindIsValid(t *testing.T) {
	t.Parallel()

	for _, kind := range []CreditKind{PayInInstallmentsCreditKind, PayNextMonthCreditKind} {
		if !kind.IsValid() {
			t.Fatalf("%q should be valid", kind)
		}
	}

	if CreditKind("pay_in_4").IsValid() {
		t.Fatal("pay_in_4 must not be a valid CreditKind")
	}

	if CreditKind("pay_in_3").IsValid() {
		t.Fatal("pay_in_3 must not be a valid CreditKind")
	}
}

func TestEffectiveCreditKindDefaultsToInstallments(t *testing.T) {
	t.Parallel()

	if got := EffectiveCreditKind(""); got != PayInInstallmentsCreditKind {
		t.Fatalf("legacy credit kind = %q, want %q", got, PayInInstallmentsCreditKind)
	}

	if got := EffectiveCreditKind(PayNextMonthCreditKind); got != PayNextMonthCreditKind {
		t.Fatalf("monthly credit kind = %q, want %q", got, PayNextMonthCreditKind)
	}
}

func TestCreditKindScanAndValue(t *testing.T) {
	t.Parallel()

	var unset CreditKind
	if err := unset.Scan(nil); err != nil {
		t.Fatalf("scan NULL: %v", err)
	}

	if unset != "" {
		t.Fatalf("scan NULL = %q, want empty", unset)
	}

	value, err := unset.Value()
	if err != nil {
		t.Fatalf("value empty: %v", err)
	}

	if value != nil {
		t.Fatalf("value empty = %#v, want nil", value)
	}

	var kind CreditKind
	if err := kind.Scan("pay_next_month"); err != nil {
		t.Fatalf("scan monthly: %v", err)
	}

	if kind != PayNextMonthCreditKind {
		t.Fatalf("scan monthly = %q", kind)
	}

	value, err = kind.Value()
	if err != nil {
		t.Fatalf("value monthly: %v", err)
	}

	if value != "pay_next_month" {
		t.Fatalf("value monthly = %#v", value)
	}

	if err := kind.Scan("pay_in_4"); err == nil {
		t.Fatal("scan pay_in_4 must fail")
	}
}

func TestRepaymentPlanKindIsValid(t *testing.T) {
	t.Parallel()

	for _, kind := range []RepaymentPlanKind{PayIn3RepaymentPlan, PayIn4RepaymentPlan} {
		if !kind.IsValid() {
			t.Fatalf("%q should be valid", kind)
		}
	}

	if RepaymentPlanKind("pay_in_installments").IsValid() {
		t.Fatal("pay_in_installments must not be a valid RepaymentPlanKind")
	}
}

func TestRepaymentPlanFromInstallmentCount(t *testing.T) {
	t.Parallel()

	got, err := RepaymentPlanFromInstallmentCount(3)
	if err != nil || got != PayIn3RepaymentPlan {
		t.Fatalf("count 3 = %q (%v), want %q", got, err, PayIn3RepaymentPlan)
	}

	got, err = RepaymentPlanFromInstallmentCount(4)
	if err != nil || got != PayIn4RepaymentPlan {
		t.Fatalf("count 4 = %q (%v), want %q", got, err, PayIn4RepaymentPlan)
	}

	if _, err := RepaymentPlanFromInstallmentCount(1); err == nil {
		t.Fatal("count 1 must not map to a repayment plan")
	}
}

func TestFinancingIdentityFromInstallmentCount(t *testing.T) {
	t.Parallel()

	kind, plan, err := FinancingIdentityFromInstallmentCount(3)
	if err != nil || kind != PayInInstallmentsCreditKind || plan != PayIn3RepaymentPlan {
		t.Fatalf("count 3 = %q/%q (%v)", kind, plan, err)
	}

	kind, plan, err = FinancingIdentityFromInstallmentCount(4)
	if err != nil || kind != PayInInstallmentsCreditKind || plan != PayIn4RepaymentPlan {
		t.Fatalf("count 4 = %q/%q (%v)", kind, plan, err)
	}

	kind, plan, err = FinancingIdentityFromInstallmentCount(1)
	if err != nil || kind != PayNextMonthCreditKind || plan != "" {
		t.Fatalf("count 1 = %q/%q (%v)", kind, plan, err)
	}
}
