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

func TestListBranches_MissingMerchantID(t *testing.T) {
	t.Parallel()

	uc := merchantuc.New(nil, nil, nil, nil)
	_, err := uc.ListBranches(context.Background(), merchantuc.ListBranchesInput{
		Mode: merchantuc.PaginationModeKeyset,
	})
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestListBranches_Keyset_EmptyWhenMerchantMissing(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)
	branchProfile := mock.NewMerchantBranchProfile(t)

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

	uc := merchantuc.New(pool, merchantProfile, branchProfile, nil)

	out, err := uc.ListBranches(context.Background(), merchantuc.ListBranchesInput{
		MerchantID: merchantID,
		Mode:       merchantuc.PaginationModeKeyset,
		Keyset:     pagination.KeysetRequest{Limit: 20},
	})
	require.NoError(t, err)
	require.Empty(t, out.Branches)
	require.NotNil(t, out.Keyset)
	require.False(t, out.Keyset.HasNext)
}

func TestListBranches_Keyset_ReturnsBranches(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)
	branchProfile := mock.NewMerchantBranchProfile(t)
	branchQ := mock.NewMerchantBranchProfileConnQueryer(t)

	merchantID := model.MerchantID(uuid.New())
	branchProfileID := model.MerchantBranchProfileID(uuid.New())
	createdAt := time.Date(
		2026,
		8,
		11,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	merchantProfile.EXPECT().Conn(testifymock.Anything).Return(merchantProfileQ)
	merchantProfileQ.EXPECT().
		GetByMerchantID(testifymock.Anything, merchantID).
		Return(model.MerchantProfile{ID: model.MerchantProfileID(uuid.New()), MerchantID: merchantID}, nil)
	branchProfile.EXPECT().Conn(testifymock.Anything).Return(branchQ)
	branchQ.EXPECT().
		ListByMerchantIDKeyset(
			testifymock.Anything,
			merchantID,
			testifymock.MatchedBy(func(page pagination.KeysetRequest) bool {
				return page.Limit == 21 && page.Order == pagination.OrderDesc
			}),
		).
		Return([]model.MerchantBranchProfile{{
			ID:         branchProfileID,
			MerchantID: merchantID,
			BranchID:   model.BranchID(uuid.New()),
			Type:       model.PhysicalMerchantBranchType,
			Status:     model.ActiveMerchantBranchStatus,
			Currencies: []model.Currency{model.IRRCurrency},
			CreatedAt:  createdAt,
			UpdatedAt:  createdAt,
		}}, nil)

	uc := merchantuc.New(pool, merchantProfile, branchProfile, nil)

	out, err := uc.ListBranches(context.Background(), merchantuc.ListBranchesInput{
		MerchantID: merchantID,
		Mode:       merchantuc.PaginationModeKeyset,
		Keyset:     pagination.KeysetRequest{Limit: 20},
	})
	require.NoError(t, err)
	require.Len(t, out.Branches, 1)
	require.Equal(t, branchProfileID, out.Branches[0].ID)
	require.Equal(t, model.ActiveMerchantBranchStatus, out.Branches[0].Status)
	require.NotNil(t, out.Keyset)
	require.False(t, out.Keyset.HasNext)
}

func TestListBranches_Page_ReturnsTotals(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)
	branchProfile := mock.NewMerchantBranchProfile(t)
	branchQ := mock.NewMerchantBranchProfileConnQueryer(t)

	merchantID := model.MerchantID(uuid.New())

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	merchantProfile.EXPECT().Conn(testifymock.Anything).Return(merchantProfileQ)
	merchantProfileQ.EXPECT().
		GetByMerchantID(testifymock.Anything, merchantID).
		Return(model.MerchantProfile{ID: model.MerchantProfileID(uuid.New()), MerchantID: merchantID}, nil)
	branchProfile.EXPECT().Conn(testifymock.Anything).Return(branchQ).Once()
	branchQ.EXPECT().
		CountByMerchantID(testifymock.Anything, merchantID).
		Return(int64(3), nil)
	branchQ.EXPECT().
		ListByMerchantIDOffset(
			testifymock.Anything,
			merchantID,
			testifymock.MatchedBy(func(page pagination.OffsetRequest) bool {
				return page.Page == 1 && page.Limit == 20
			}),
		).
		Return([]model.MerchantBranchProfile{{
			ID:         model.MerchantBranchProfileID(uuid.New()),
			MerchantID: merchantID,
			BranchID:   model.BranchID(uuid.New()),
			Type:       model.PhysicalMerchantBranchType,
			Status:     model.SuspendedMerchantBranchStatus,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}}, nil)

	uc := merchantuc.New(pool, merchantProfile, branchProfile, nil)

	out, err := uc.ListBranches(context.Background(), merchantuc.ListBranchesInput{
		MerchantID: merchantID,
		Mode:       merchantuc.PaginationModePage,
		Offset: pagination.OffsetRequest{
			Page:  1,
			Limit: 20,
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Branches, 1)
	require.Equal(t, model.SuspendedMerchantBranchStatus, out.Branches[0].Status)
	require.NotNil(t, out.Page)
	require.Equal(t, int64(3), out.Page.Total)
}
