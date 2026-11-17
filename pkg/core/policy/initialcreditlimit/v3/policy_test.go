package v3

import (
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	installment "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
)

func TestMonthlyInitialCreditLimitIsIndependent(t *testing.T){
	monthly:=New().Terms()
	payInFour:=installment.New().Terms()
	if monthly.Currency!=model.IRRCurrency{
		t.Fatalf("monthly currency=%s, want IRR",monthly.Currency)
	}
	if monthly.DefaultAmountMinor!=30_000_000{
		t.Fatalf("monthly default=%d, want 30000000 IRR",monthly.DefaultAmountMinor)
	}
	if payInFour.DefaultAmountMinor!=100_000_000{
		t.Fatalf("pay-in-four default=%d, want 100000000 IRR",payInFour.DefaultAmountMinor)
	}
	if monthly.DefaultAmountMinor==payInFour.DefaultAmountMinor{
		t.Fatal("monthly and pay-in-four initial credit limits unexpectedly match")
	}
}
