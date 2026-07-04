package v1_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
)

var fixtureNow = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

func eligibleEligibilityInput() checkout.EligibilityInput {
	expiresAt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	return checkout.EligibilityInput{
		Now:                  fixtureNow,
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
		BranchRequested:      false,
		CustomerProfile: model.CustomerProfile{
			ID:                model.CustomerID(uuid.New()),
			IdentitySubjectID: model.IdentitySubjectID(uuid.New()),
			Jurisdiction:      model.IRJurisdiction,
			Status:            model.ActiveCustomerStatus,
		},
		MerchantProfile: model.MerchantProfile{
			ID:                model.MerchantProfileID(uuid.New()),
			MerchantID:        model.MerchantID(uuid.New()),
			Status:            model.ActiveMerchantStatus,
			Currencies:        []model.Currency{model.IRRCurrency},
			MaxAmountMinor:    1_000_000,
			MaxAmountCurrency: model.IRRCurrency,
			MaxTenureMonths:   12,
			Jurisdiction:      model.IRJurisdiction,
		},
		CreditAccount: model.CreditAccount{
			LimitAmountMinor: 500_000,
			Currency:         model.IRRCurrency,
			Status:           model.ActiveCreditAccountStatus,
			ExpiresAt:        &expiresAt,
		},
		HasCreditAccount: true,
	}
}

func TestEvaluateEligibility_Approved(t *testing.T) {
	t.Parallel()

	policy := v1.New()
	result := policy.EvaluateEligibility(eligibleEligibilityInput())

	require.Equal(t, model.ApprovedDecisionOutcome, result.Outcome)
	require.Empty(t, result.ReasonCodes)
	require.Equal(t, model.IRJurisdiction, result.Jurisdiction)
}

func TestEvaluateEligibility_DenialBranches(t *testing.T) {
	t.Parallel()

	otherMerchant := model.MerchantID(uuid.New())
	requestedBranchID := model.MerchantBranchProfileID(uuid.New())

	tests := []struct {
		name       string
		mutate     func(*checkout.EligibilityInput)
		wantReason string
	}{
		{
			name: "inactive customer profile",
			mutate: func(in *checkout.EligibilityInput) {
				in.CustomerProfile.Status = model.DeactivatedCustomerStatus
			},
			wantReason: model.CustomerProfileNotActiveDecisionReason,
		},
		{
			name: "inactive merchant profile",
			mutate: func(in *checkout.EligibilityInput) {
				in.MerchantProfile.Status = model.SuspendedMerchantStatus
			},
			wantReason: model.MerchantProfileNotActiveDecisionReason,
		},
		{
			name: "branch required but missing",
			mutate: func(in *checkout.EligibilityInput) {
				in.MerchantProfile.RequiresBranchMode = true
				in.BranchRequested = false
			},
			wantReason: model.MerchantBranchProfileRequiredDecisionReason,
		},
		{
			name: "branch requested but not found",
			mutate: func(in *checkout.EligibilityInput) {
				in.BranchRequested = true
				in.MerchantBranchProfile = nil
			},
			wantReason: model.MerchantBranchProfileNotActiveDecisionReason,
		},
		{
			name: "branch belongs to another merchant",
			mutate: func(in *checkout.EligibilityInput) {
				in.BranchRequested = true
				in.MerchantBranchProfile = &model.MerchantBranchProfile{
					ID:         requestedBranchID,
					MerchantID: otherMerchant,
					Status:     model.ActiveMerchantBranchStatus,
				}
			},
			wantReason: model.MerchantBranchDoesNotBelongToMerchantDecisionReason,
		},
		{
			name: "branch inactive",
			mutate: func(in *checkout.EligibilityInput) {
				in.BranchRequested = true
				in.MerchantBranchProfile = &model.MerchantBranchProfile{
					ID:         requestedBranchID,
					MerchantID: in.MerchantProfile.MerchantID,
					Status:     model.DeactivatedMerchantBranchStatus,
				}
			},
			wantReason: model.MerchantBranchProfileNotActiveDecisionReason,
		},
		{
			name: "branch jurisdiction mismatch",
			mutate: func(in *checkout.EligibilityInput) {
				in.BranchRequested = true
				in.MerchantBranchProfile = &model.MerchantBranchProfile{
					ID:           requestedBranchID,
					MerchantID:   in.MerchantProfile.MerchantID,
					Status:       model.ActiveMerchantBranchStatus,
					Jurisdiction: model.Jurisdiction("US"),
				}
			},
			wantReason: model.JurisdictionMismatchDecisionReason,
		},
		{
			name: "merchant jurisdiction mismatch",
			mutate: func(in *checkout.EligibilityInput) {
				in.MerchantProfile.Jurisdiction = model.Jurisdiction("US")
			},
			wantReason: model.JurisdictionMismatchDecisionReason,
		},
		{
			name: "currency not supported",
			mutate: func(in *checkout.EligibilityInput) {
				in.MerchantProfile.Currencies = []model.Currency{model.USDCurrency}
			},
			wantReason: model.CurrencyNotSupportedDecisionReason,
		},
		{
			name: "branch currency override not supported",
			mutate: func(in *checkout.EligibilityInput) {
				in.BranchRequested = true
				in.MerchantBranchProfile = &model.MerchantBranchProfile{
					ID:         requestedBranchID,
					MerchantID: in.MerchantProfile.MerchantID,
					Status:     model.ActiveMerchantBranchStatus,
					Currencies: []model.Currency{model.USDCurrency},
				}
			},
			wantReason: model.CurrencyNotSupportedDecisionReason,
		},
		{
			name: "installment count exceeds tenure",
			mutate: func(in *checkout.EligibilityInput) {
				in.MerchantProfile.MaxTenureMonths = 1
			},
			wantReason: model.InstallmentCountNotSupportedDecisionReason,
		},
		{
			name: "branch tenure override exceeded",
			mutate: func(in *checkout.EligibilityInput) {
				in.BranchRequested = true
				override := uint(1)
				in.MerchantBranchProfile = &model.MerchantBranchProfile{
					ID:                      requestedBranchID,
					MerchantID:              in.MerchantProfile.MerchantID,
					Status:                  model.ActiveMerchantBranchStatus,
					MaxTenureMonthsOverride: &override,
				}
			},
			wantReason: model.InstallmentCountNotSupportedDecisionReason,
		},
		{
			name: "merchant amount currency mismatch",
			mutate: func(in *checkout.EligibilityInput) {
				in.MerchantProfile.MaxAmountCurrency = model.USDCurrency
			},
			wantReason: model.MerchantAmountCurrencyMismatchDecisionReason,
		},
		{
			name: "requested amount exceeds merchant limit",
			mutate: func(in *checkout.EligibilityInput) {
				in.MerchantProfile.MaxAmountMinor = 1
			},
			wantReason: model.RequestedAmountExceedsMerchantLimitDecisionReason,
		},
		{
			name: "branch amount override exceeded",
			mutate: func(in *checkout.EligibilityInput) {
				in.BranchRequested = true
				overrideAmount := uint64(1)
				overrideCurrency := model.IRRCurrency
				in.MerchantBranchProfile = &model.MerchantBranchProfile{
					ID:                        requestedBranchID,
					MerchantID:                in.MerchantProfile.MerchantID,
					Status:                    model.ActiveMerchantBranchStatus,
					MaxAmountOverrideMinor:    &overrideAmount,
					MaxAmountOverrideCurrency: &overrideCurrency,
				}
			},
			wantReason: model.RequestedAmountExceedsMerchantLimitDecisionReason,
		},
		{
			name: "credit limit missing",
			mutate: func(in *checkout.EligibilityInput) {
				in.HasCreditAccount = false
				in.CreditAccount = model.CreditAccount{}
			},
			wantReason: model.CreditLimitMissingDecisionReason,
		},
		{
			name: "credit limit not active",
			mutate: func(in *checkout.EligibilityInput) {
				in.CreditAccount.Status = model.RevokedCreditAccountStatus
			},
			wantReason: model.CreditLimitNotActiveDecisionReason,
		},
		{
			name: "credit limit currency mismatch",
			mutate: func(in *checkout.EligibilityInput) {
				in.CreditAccount.Currency = model.USDCurrency
			},
			wantReason: model.CurrencyNotSupportedDecisionReason,
		},
		{
			name: "credit limit expired at exact boundary",
			mutate: func(in *checkout.EligibilityInput) {
				expiresAt := in.Now
				in.CreditAccount.ExpiresAt = &expiresAt
			},
			wantReason: model.CreditLimitExpiredDecisionReason,
		},
		{
			name: "credit limit expired in the past",
			mutate: func(in *checkout.EligibilityInput) {
				expiresAt := in.Now.Add(-time.Hour)
				in.CreditAccount.ExpiresAt = &expiresAt
			},
			wantReason: model.CreditLimitExpiredDecisionReason,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := eligibleEligibilityInput()
			tt.mutate(&input)

			result := v1.New().EvaluateEligibility(input)

			require.Equal(t, model.DeniedDecisionOutcome, result.Outcome)
			require.Equal(t, []string{tt.wantReason}, result.ReasonCodes)
		})
	}
}

func TestEvaluateEligibility_CreditLimitExpiresAfterNow_Approved(t *testing.T) {
	t.Parallel()

	input := eligibleEligibilityInput()
	expiresAt := input.Now.Add(time.Nanosecond)
	input.CreditAccount.ExpiresAt = &expiresAt

	result := v1.New().EvaluateEligibility(input)

	require.Equal(t, model.ApprovedDecisionOutcome, result.Outcome)
}

func TestEvaluateEligibility_BranchOverridesGrantWiderLimits(t *testing.T) {
	t.Parallel()

	input := eligibleEligibilityInput()
	input.MerchantProfile.MaxAmountMinor = 1
	input.MerchantProfile.RequiresBranchMode = true
	input.BranchRequested = true

	overrideAmount := uint64(1_000_000)
	overrideCurrency := model.IRRCurrency
	input.MerchantBranchProfile = &model.MerchantBranchProfile{
		ID:                        model.MerchantBranchProfileID(uuid.New()),
		MerchantID:                input.MerchantProfile.MerchantID,
		Status:                    model.ActiveMerchantBranchStatus,
		MaxAmountOverrideMinor:    &overrideAmount,
		MaxAmountOverrideCurrency: &overrideCurrency,
	}

	result := v1.New().EvaluateEligibility(input)

	require.Equal(t, model.ApprovedDecisionOutcome, result.Outcome)
}

func TestEvaluateEligibility_DeterministicForIdenticalInput(t *testing.T) {
	t.Parallel()

	input := eligibleEligibilityInput()
	policy := v1.New()

	first := policy.EvaluateEligibility(input)
	second := policy.EvaluateEligibility(input)

	require.Equal(t, first, second)
}
