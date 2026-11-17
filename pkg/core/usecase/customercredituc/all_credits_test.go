package customercredituc

import (
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestAggregateCreditStatus(t *testing.T) {
	cases := []struct {
		name  string
		items []Output
		want  AllCreditStatus
	}{
		{
			name: "neither assigned",
			items: []Output{
				{CreditKind: model.PayIn4CreditKind, Status: SummaryStatusNotAssigned},
				{CreditKind: model.PayNextMonthCreditKind, Status: SummaryStatusNotAssigned},
			},
			want: AllCreditNotAssigned,
		},
		{
			name: "only installments assigned",
			items: []Output{
				{CreditKind: model.PayIn4CreditKind, Status: SummaryStatusActive},
				{CreditKind: model.PayNextMonthCreditKind, Status: SummaryStatusNotAssigned},
			},
			want: AllCreditPartiallyAssigned,
		},
		{
			name: "expired is not silently omitted",
			items: []Output{
				{CreditKind: model.PayIn4CreditKind, Status: SummaryStatusExpired},
				{CreditKind: model.PayNextMonthCreditKind, Status: SummaryStatusNotAssigned},
			},
			want: AllCreditPartiallyAssigned,
		},
		{
			name: "both assigned",
			items: []Output{
				{CreditKind: model.PayIn4CreditKind, Status: SummaryStatusActive},
				{CreditKind: model.PayNextMonthCreditKind, Status: SummaryStatusActive},
			},
			want: AllCreditAssigned,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AggregateCreditStatus(tc.items)
			if got != tc.want {
				t.Fatalf("AggregateCreditStatus()=%q, want %q", got, tc.want)
			}
		})
	}
}
