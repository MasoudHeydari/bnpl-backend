package model

import "testing"

func TestCreditKindValidityAndIsolation(t *testing.T) {
	for _, kind := range []CreditKind{PayIn4CreditKind, PayNextMonthCreditKind} {
		if !kind.IsValid() {
			t.Fatalf("valid credit kind %q rejected", kind)
		}
	}

	if CreditKind("unknown").IsValid() || CreditKind("").IsValid() {
		t.Fatal("invalid product kind accepted")
	}

	if got := EffectiveCreditKind(""); got != PayIn4CreditKind {
		t.Fatalf("legacy credit kind = %q, want %q", got, PayIn4CreditKind)
	}

	if got := EffectiveCreditKind(PayNextMonthCreditKind); got != PayNextMonthCreditKind {
		t.Fatalf("explicit credit kind changed to %q", got)
	}
}
