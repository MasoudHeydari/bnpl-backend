package customeraccountuc

import (
	"context"
	"github.com/google/uuid"
	coremock "github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSummaryUsesGlobalPriorityAndFinancialRecognition(t *testing.T) {
	for _, tc := range []struct {
		name    string
		charges bool
		pending bool
	}{
		{name: "principal recognized"}, {name: "unrecognized fee", charges: true}, {name: "pending priority is retained", pending: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			customer := model.CustomerID(uuid.New())
			contract := model.ContractID(uuid.New())
			now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
			i := model.Installment{ID: model.InstallmentID(uuid.New()), ContractID: &contract, PrincipalAmountMinor: 100, TotalAmountMinor: 100, Currency: model.IRRCurrency, Status: model.DueInstallmentStatus, DueAt: now.Add(-24 * time.Hour)}
			iq := coremock.NewInstallmentQueryer(t)
			pq := coremock.NewPaymentAttemptQueryer(t)
			cq := coremock.NewInstallmentChargeQueryer(t)
			// The summary works across a customer with more than 200 obligations. It
			// consumes global repository candidates independently of any page contents.
			iq.EXPECT().CountUnpaidByCustomerID(ctx, customer).Return(1000, nil)
			iq.EXPECT().ListDueUnpaidByCustomerID(ctx, customer, now).Return([]model.Installment{i}, nil)
			iq.EXPECT().ListNextPayablePerContractByCustomerID(ctx, customer).Return([]model.Installment{i}, nil)
			var pending []model.InstallmentID
			if tc.pending {
				pending = []model.InstallmentID{i.ID}
			}
			pq.EXPECT().ListInstallmentIDsWithNonTerminalAttempts(ctx, customer, mock.Anything).Return(pending, nil)
			var charges []model.InstallmentCharge
			if tc.charges {
				charges = []model.InstallmentCharge{{ID: model.InstallmentChargeID(uuid.New()), InstallmentID: i.ID, ChargeType: model.BNPLFeeInstallmentChargeType, ChargeStatus: model.AppliedInstallmentChargeStatus, PayerType: model.CustomerInstallmentChargePayerType, AmountMinor: 20}}
			}
			cq.EXPECT().ListByInstallmentIDs(ctx, []model.InstallmentID{i.ID}).Return(charges, nil)
			got, err := buildRepaymentSummary(ctx, iq, pq, cq, customer, now)
			require.NoError(t, err)
			require.Equal(t, 1000, got.UnpaidCount)
			require.NotNil(t, got.NextPayable)
			require.Equal(t, i.ID, got.NextPayable.InstallmentID)
			require.Equal(t, tc.pending, got.HasPendingPayment)
			require.Equal(t, !tc.charges, got.NextPayable.PayableAmountKnown)
			if tc.charges {
				require.Zero(t, got.NextPayable.PayableAmountMinor)
			} else {
				require.EqualValues(t, 100, got.NextPayable.PayableAmountMinor)
			}
		})
	}
}
