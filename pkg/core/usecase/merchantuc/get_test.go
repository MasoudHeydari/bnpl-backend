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
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

func TestGetProfile_MissingMerchantID(t *testing.T) {
	t.Parallel()

	uc := merchantuc.New(nil, nil, nil, nil)
	_, err := uc.GetProfile(context.Background(), merchantuc.GetProfileInput{})
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))

	var ce *cerr.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, cerr.MessageKey("error.merchant_id_required"), ce.MessageKey)
	require.Equal(t, []string{"merchant_id"}, ce.Details.Fields)
	require.Len(t, ce.Details.Violations, 1)
	require.Equal(t, "REQUIRED", ce.Details.Violations[0].Code)
}

func TestGetProfile_NotFound(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)

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

	uc := merchantuc.New(pool, merchantProfile, nil, nil)

	_, err := uc.GetProfile(context.Background(), merchantuc.GetProfileInput{MerchantID: merchantID})
	require.True(t, cerr.IsCode(err, cerr.CodeResourceNotFound))

	var ce *cerr.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, cerr.MessageKey("error.merchant_profile_not_found"), ce.MessageKey)
	require.Empty(t, ce.Details.Fields)
	require.Len(t, ce.Details.Violations, 1)
	require.Equal(t, "NOT_FOUND", ce.Details.Violations[0].Code)
	require.Equal(t, cerr.MessageKey("error.no_bnpl_merchant_profile_exists_for_this_merchant"), ce.Details.Violations[0].MessageKey)
}

func TestGetProfile_ReturnsProfile(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantProfileQ := mock.NewMerchantProfileConnQueryer(t)

	merchantID := model.MerchantID(uuid.New())
	createdAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	merchantProfile.EXPECT().Conn(testifymock.Anything).Return(merchantProfileQ)
	merchantProfileQ.EXPECT().
		GetByMerchantID(testifymock.Anything, merchantID).
		Return(model.MerchantProfile{
			ID:                 model.MerchantProfileID(uuid.New()),
			MerchantID:         merchantID,
			DisplayName:        "Demo Merchant",
			LegalName:          "Demo Merchant LLC",
			LogoURL:            "https://cdn.example/logo.png",
			SupportPhone:       "+989121234567",
			SupportEmail:       "support@example.com",
			Status:             model.ActiveMerchantStatus,
			FundingModels:      []model.FundingModel{model.PlatformFundedFundingModel},
			SettlementModels:   []model.SettlementModel{model.DirectSettlementModel},
			Currencies:         []model.Currency{model.IRRCurrency},
			RiskTier:           model.LowRiskTier,
			MaxAmountMinor:     50_000_000,
			MaxAmountCurrency:  model.IRRCurrency,
			MaxTenureMonths:    3,
			RequiresBranchMode: false,
			Jurisdiction:       model.IRJurisdiction,
			CreatedAt:          createdAt,
			UpdatedAt:          createdAt,
		}, nil)

	uc := merchantuc.New(pool, merchantProfile, nil, nil)

	out, err := uc.GetProfile(context.Background(), merchantuc.GetProfileInput{MerchantID: merchantID})
	require.NoError(t, err)
	require.Equal(t, merchantID, out.MerchantID)
	require.Equal(t, "Demo Merchant", out.DisplayName)
	require.Equal(t, "Demo Merchant LLC", out.LegalName)
	require.Equal(t, "https://cdn.example/logo.png", out.LogoURL)
	require.Equal(t, "+989121234567", out.SupportPhone)
	require.Equal(t, "support@example.com", out.SupportEmail)
	require.Equal(t, model.ActiveMerchantStatus, out.Status)
	require.Equal(t, uint64(50_000_000), out.MaxAmountMinor)
	require.Equal(t, model.IRRCurrency, out.MaxAmountCurrency)
	require.Equal(t, uint(3), out.MaxTenureMonths)
	require.False(t, out.RequiresBranchMode)
	require.Equal(t, model.IRJurisdiction, out.Jurisdiction)
}
