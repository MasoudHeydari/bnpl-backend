package merchantuc_test

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
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

func TestListSales_MissingMerchantID(t *testing.T) {
	t.Parallel()

	uc := merchantuc.New(nil, nil, nil, nil)
	_, err := uc.ListSales(context.Background(), merchantuc.ListSalesInput{
		Mode: merchantuc.PaginationModeKeyset,
	})
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestListSales_InvalidPaginationMode(t *testing.T) {
	t.Parallel()

	uc := merchantuc.New(nil, nil, nil, nil)
	_, err := uc.ListSales(context.Background(), merchantuc.ListSalesInput{
		MerchantID: model.MerchantID(uuid.New()),
		Mode:       "cursor",
	})
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestListSales_Keyset_EmptyWhenMerchantMissing(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)
	contract := mock.NewContract(t)

	merchantID := model.MerchantID(uuid.New())

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	merchantProfile.EXPECT().Conn(testifymock.Anything).Return(merchantProfileQ)
	merchantProfileQ.EXPECT().
		GetByMerchantID(testifymock.Anything, merchantID).
		Return(model.MerchantProfile{}, cerr.ResourceNotFound(errors.New("missing")))

	uc := merchantuc.New(pool, merchantProfile, nil, contract)

	out, err := uc.ListSales(context.Background(), merchantuc.ListSalesInput{
		MerchantID: merchantID,
		Mode:       merchantuc.PaginationModeKeyset,
		Keyset:     pagination.KeysetRequest{Limit: 20},
	})
	require.NoError(t, err)
	require.Empty(t, out.Sales)
	require.NotNil(t, out.Keyset)
	require.False(t, out.Keyset.HasNext)
	require.Equal(t, 20, out.Keyset.Limit)
	require.Nil(t, out.Page)
}

func TestListSales_Keyset_ReturnsActiveSales(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)
	contract := mock.NewContract(t)
	contractQ := mock.NewContractConnQueryer(t)

	merchantID := model.MerchantID(uuid.New())
	profileID := model.MerchantProfileID(uuid.New())
	saleID := model.ContractID(uuid.New())
	createdAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	merchantProfile.EXPECT().Conn(testifymock.Anything).Return(merchantProfileQ)
	merchantProfileQ.EXPECT().
		GetByMerchantID(testifymock.Anything, merchantID).
		Return(model.MerchantProfile{ID: profileID, MerchantID: merchantID}, nil)
	contract.EXPECT().Conn(testifymock.Anything).Return(contractQ)
	contractQ.EXPECT().
		ListActiveByMerchantProfileID(
			testifymock.Anything,
			profileID,
			testifymock.MatchedBy(func(page pagination.KeysetRequest) bool {
				return page.Limit == 21 && page.Order == pagination.OrderDesc
			}),
		).
		Return([]model.Contract{{
			ID:                            saleID,
			CustomerID:                    model.CustomerID(uuid.New()),
			MerchantProfileID:             profileID,
			RequestedAmountMinor:          1_500_000,
			RemainingPrincipalAmountMinor: 1_000_000,
			Currency:                      model.IRRCurrency,
			InstallmentCount:              3,
			Status:                        model.ActiveContractStatus,
			CreatedAt:                     createdAt,
		}}, nil)

	uc := merchantuc.New(pool, merchantProfile, nil, contract)

	out, err := uc.ListSales(context.Background(), merchantuc.ListSalesInput{
		MerchantID: merchantID,
		Mode:       merchantuc.PaginationModeKeyset,
		Keyset:     pagination.KeysetRequest{Limit: 20},
	})
	require.NoError(t, err)
	require.Len(t, out.Sales, 1)
	require.Equal(t, saleID, out.Sales[0].ID)
	require.Equal(t, uint64(1_500_000), out.Sales[0].RequestedAmountMinor)
	require.NotNil(t, out.Keyset)
	require.False(t, out.Keyset.HasNext)
}

func TestListSales_Page_ReturnsTotals(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)
	contract := mock.NewContract(t)
	contractQ := mock.NewContractConnQueryer(t)

	merchantID := model.MerchantID(uuid.New())
	profileID := model.MerchantProfileID(uuid.New())

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	merchantProfile.EXPECT().Conn(testifymock.Anything).Return(merchantProfileQ)
	merchantProfileQ.EXPECT().
		GetByMerchantID(testifymock.Anything, merchantID).
		Return(model.MerchantProfile{ID: profileID, MerchantID: merchantID}, nil)
	contract.EXPECT().Conn(testifymock.Anything).Return(contractQ).Once()
	contractQ.EXPECT().
		CountActiveByMerchantProfileID(testifymock.Anything, profileID).
		Return(int64(2), nil)
	contractQ.EXPECT().
		ListActiveByMerchantProfileIDOffset(
			testifymock.Anything,
			profileID,
			testifymock.MatchedBy(func(page pagination.OffsetRequest) bool {
				return page.Page == 1 && page.Limit == 20 && page.Order == pagination.OrderDesc
			}),
		).
		Return([]model.Contract{{
			ID:                   model.ContractID(uuid.New()),
			CustomerID:           model.CustomerID(uuid.New()),
			MerchantProfileID:    profileID,
			RequestedAmountMinor: 500_000,
			Currency:             model.IRRCurrency,
			InstallmentCount:     2,
			Status:               model.ActiveContractStatus,
			CreatedAt:            time.Now().UTC(),
		}}, nil)

	uc := merchantuc.New(pool, merchantProfile, nil, contract)

	out, err := uc.ListSales(context.Background(), merchantuc.ListSalesInput{
		MerchantID: merchantID,
		Mode:       merchantuc.PaginationModePage,
		Offset: pagination.OffsetRequest{
			Page:  1,
			Limit: 20,
			Order: pagination.OrderDesc,
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Sales, 1)
	require.NotNil(t, out.Page)
	require.Equal(t, int64(2), out.Page.Total)
	require.Nil(t, out.Keyset)
}
