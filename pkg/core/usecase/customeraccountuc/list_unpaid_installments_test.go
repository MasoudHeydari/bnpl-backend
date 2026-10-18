package customeraccountuc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
)

func TestListUnpaidInstallments_EmptyWhenCustomerProfileMissing(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	customerProfile := mock.NewCustomerProfile(t)
	customerProfileQ := mock.NewCustomerProfileConnQueryer(t)
	installment := mock.NewInstallment(t)

	externalCustomerID := model.ExternalCustomerID(uuid.New())

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	customerProfile.EXPECT().Conn(testifymock.Anything).Return(customerProfileQ)
	customerProfileQ.EXPECT().
		GetByExternalCustomerID(testifymock.Anything, externalCustomerID).
		Return(model.CustomerProfile{}, cerr.ResourceNotFound(errors.New("missing")))

	uc := customeraccountuc.New(pool, customerProfile, nil, installment, nil, nil)

	out, err := uc.ListUnpaidInstallments(context.Background(), customeraccountuc.ListUnpaidInstallmentsInput{
		ExternalCustomerID: externalCustomerID,
		Page:              pagination.KeysetRequest{Limit: 20},
	})
	require.NoError(t, err)
	require.Empty(t, out.Installments)
	require.False(t, out.Page.HasNext)
	require.Equal(t, 20, out.Page.Limit)
}

func TestListUnpaidInstallments_OrdersByDueAtAndBuildsDueAtCursor(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	customerProfile := mock.NewCustomerProfile(t)
	customerProfileQ := mock.NewCustomerProfileConnQueryer(t)
	installment := mock.NewInstallment(t)
	installmentQ := mock.NewInstallmentConnQueryer(t)

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	profileID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	due1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	due2 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	id1 := model.InstallmentID(uuid.New())
	id2 := model.InstallmentID(uuid.New())

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	customerProfile.EXPECT().Conn(testifymock.Anything).Return(customerProfileQ)
	customerProfileQ.EXPECT().
		GetByExternalCustomerID(testifymock.Anything, externalCustomerID).
		Return(model.CustomerProfile{ID: profileID, ExternalCustomerID: externalCustomerID}, nil)
	installment.EXPECT().Conn(testifymock.Anything).Return(installmentQ)
	installmentQ.EXPECT().
		ListUnpaidByCustomerID(testifymock.Anything, profileID, testifymock.MatchedBy(func(page pagination.KeysetRequest) bool {
			return page.Limit == 2 && page.Order == pagination.OrderAsc
		})).
		Return([]model.Installment{
			{
				ID:                id1,
				ContractID:        &contractID,
				MerchantProfileID: model.MerchantProfileID(uuid.New()),
				InstallmentOrder:  1,
				TotalAmountMinor:  1000,
				Currency:          model.IRRCurrency,
				DueAt:             due1,
				Status:            model.DueInstallmentStatus,
			},
			{
				ID:                id2,
				ContractID:        &contractID,
				MerchantProfileID: model.MerchantProfileID(uuid.New()),
				InstallmentOrder:  2,
				TotalAmountMinor:  1000,
				Currency:          model.IRRCurrency,
				DueAt:             due2,
				Status:            model.ScheduledInstallmentStatus,
			},
		}, nil)

	uc := customeraccountuc.New(pool, customerProfile, nil, installment, nil, nil)

	out, err := uc.ListUnpaidInstallments(context.Background(), customeraccountuc.ListUnpaidInstallmentsInput{
		ExternalCustomerID: externalCustomerID,
		Page:              pagination.KeysetRequest{Limit: 1},
	})
	require.NoError(t, err)
	require.Len(t, out.Installments, 1)
	require.Equal(t, id1, out.Installments[0].ID)
	require.True(t, out.Page.HasNext)
	require.NotNil(t, out.Page.NextAfterID)
	require.Equal(t, id1.String(), *out.Page.NextAfterID)
	require.NotNil(t, out.Page.NextAfterDueAt)
	require.Equal(t, due1.Format(time.RFC3339Nano), *out.Page.NextAfterDueAt)
	require.Nil(t, out.Page.NextAfterCreatedAt)
}
