package customeraccountuc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
)

func TestGetRepaymentOverview_PendingPaymentFromPaymentAttempts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	profileID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	pendingID := model.InstallmentID(uuid.New())
	clearID := model.InstallmentID(uuid.New())

	pool := mock.NewPool(t)
	customerProfile := mock.NewCustomerProfile(t)
	customerProfileQ := mock.NewCustomerProfileConnQueryer(t)
	installment := mock.NewInstallment(t)
	installmentQ := mock.NewInstallmentConnQueryer(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptQ := mock.NewPaymentAttemptConnQueryer(t)

	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	customerProfile.EXPECT().Conn(testifymock.Anything).Return(customerProfileQ)
	customerProfileQ.EXPECT().
		GetByExternalCustomerID(testifymock.Anything, externalCustomerID).
		Return(model.CustomerProfile{ID: profileID, ExternalCustomerID: externalCustomerID}, nil)

	installment.EXPECT().Conn(testifymock.Anything).Return(installmentQ)
	installmentQ.EXPECT().
		ListUnpaidByCustomerID(testifymock.Anything, profileID, testifymock.Anything).
		Return([]model.Installment{
			{
				ID:               pendingID,
				ContractID:       &contractID,
				CustomerID:       profileID,
				InstallmentOrder: 1,
				TotalAmountMinor: 1000,
				Currency:         model.IRRCurrency,
				DueAt:            now.Add(-48 * time.Hour),
				Status:           model.DueInstallmentStatus,
			},
			{
				ID:               clearID,
				ContractID:       &contractID,
				CustomerID:       profileID,
				InstallmentOrder: 2,
				TotalAmountMinor: 2000,
				Currency:         model.IRRCurrency,
				DueAt:            now.Add(72 * time.Hour),
				Status:           model.ScheduledInstallmentStatus,
			},
		}, nil)

	paymentAttempt.EXPECT().Conn(testifymock.Anything).Return(paymentAttemptQ)
	paymentAttemptQ.EXPECT().
		ListInstallmentIDsWithNonTerminalAttempts(testifymock.Anything, profileID, testifymock.Anything).
		RunAndReturn(func(
			_ context.Context,
			_ model.CustomerID,
			ids []model.InstallmentID,
		) ([]model.InstallmentID, error) {
			require.ElementsMatch(t, []model.InstallmentID{pendingID, clearID}, ids)
			return []model.InstallmentID{pendingID}, nil
		})

	uc, err := customeraccountuc.New(
		pool,
		customerProfile,
		mock.NewContract(t),
		installment,
		mock.NewInstallmentCharge(t),
		paymentAttempt,
		adapterclock.NewFixed(now, time.UTC),
	)
	require.NoError(t, err)

	out, err := uc.GetRepaymentOverview(context.Background(), customeraccountuc.GetRepaymentOverviewInput{
		ExternalCustomerID: externalCustomerID,
	})
	require.NoError(t, err)
	require.Len(t, out.Installments, 2)

	byID := map[model.InstallmentID]customeraccountuc.RepaymentOverviewInstallment{}
	for _, row := range out.Installments {
		byID[row.ID] = row
	}

	require.True(t, byID[pendingID].HasPendingPayment)
	require.False(t, byID[clearID].HasPendingPayment)
	require.NotNil(t, out.Priority)
	require.Equal(t, pendingID, out.Priority.InstallmentID)
}
