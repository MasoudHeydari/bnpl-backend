package v1_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
)

func approvedEligibility() checkout.EligibilityResult {
	return checkout.EligibilityResult{Outcome: model.ApprovedDecisionOutcome}
}

func modelCreditAccount(limitAmountMinor, exposureAmountMinor uint64) model.CreditAccount {
	return model.CreditAccount{
		LimitAmountMinor:    limitAmountMinor,
		ExposureAmountMinor: exposureAmountMinor,
		Currency:            model.IRRCurrency,
		Status:              model.ActiveCreditAccountStatus,
	}
}

func TestEvaluateFinancing_InsufficientAvailableCredit_PartialApproval(t *testing.T) {
	t.Parallel()

	result := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 100_000,
		CreditAccount:        modelCreditAccount(40_000, 0),
		Eligibility:          approvedEligibility(),
	})

	require.Equal(t, model.ConditionalDecisionOutcome, result.Outcome)
	require.Equal(t, []string{model.PartialApprovalRequiresFirstInstallmentPayDecisionReason}, result.ReasonCodes)
	require.Equal(t, uint64(40_000), result.FinancedAmountMinor)
	require.Equal(t, uint64(60_000), result.UpfrontAmountMinor)
}

func TestEvaluateFinancing_FullFinancing_Approved(t *testing.T) {
	t.Parallel()

	result := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 100_000,
		CreditAccount:        modelCreditAccount(500_000, 0),
		Eligibility:          approvedEligibility(),
	})

	require.Equal(t, model.ApprovedDecisionOutcome, result.Outcome)
	require.Equal(t, []string{model.ApprovedFullFinancingDecisionReason}, result.ReasonCodes)
	require.Equal(t, uint64(100_000), result.FinancedAmountMinor)
	require.Equal(t, uint64(0), result.UpfrontAmountMinor)
}

func TestEvaluateFinancing_NoAvailableCredit_Denied(t *testing.T) {
	t.Parallel()

	result := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 100_000,
		CreditAccount:        modelCreditAccount(50_000, 50_000),
		Eligibility:          approvedEligibility(),
	})

	require.Equal(t, model.DeniedDecisionOutcome, result.Outcome)
	require.Equal(t, []string{model.NoAvailableCreditDecisionReason}, result.ReasonCodes)
	require.Equal(t, uint64(0), result.AvailableCreditMinor)
}

func TestEvaluateFinancing_EligibilityDenied_PreservesAvailableCredit(t *testing.T) {
	t.Parallel()

	result := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 100_000_000_000,
		CreditAccount:        modelCreditAccount(5_000_000, 1_000_000),
		Eligibility: checkout.EligibilityResult{
			Outcome:     model.DeniedDecisionOutcome,
			ReasonCodes: []string{model.RequestedAmountExceedsMerchantLimitDecisionReason},
		},
	})

	require.Equal(t, model.DeniedDecisionOutcome, result.Outcome)
	require.Equal(t, []string{model.RequestedAmountExceedsMerchantLimitDecisionReason}, result.ReasonCodes)
	require.Equal(t, uint64(4_000_000), result.AvailableCreditMinor)
	require.Equal(t, uint64(0), result.FinancedAmountMinor)
}

func TestEvaluateFinancing_FinancedTooSmall_PreservesAvailableCredit(t *testing.T) {
	t.Parallel()

	result := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 3,
		CreditAccount:        modelCreditAccount(3, 0),
		Eligibility:          approvedEligibility(),
	})

	require.Equal(t, model.DeniedDecisionOutcome, result.Outcome)
	require.Equal(t, []string{model.FinancedAmountTooSmallForInstallmentCountDecisionReason}, result.ReasonCodes)
	require.Equal(t, uint64(3), result.AvailableCreditMinor)
}

func TestEvaluateFinancing_FinancedTooSmallForInstallmentCountBoundary(t *testing.T) {
	t.Parallel()

	// installmentCount() == 4: financed == 3 denies, financed == 4 passes through.
	deniedResult := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 3,
		CreditAccount:        modelCreditAccount(3, 0),
		Eligibility:          approvedEligibility(),
	})
	require.Equal(t, model.DeniedDecisionOutcome, deniedResult.Outcome)
	require.Equal(t, []string{model.FinancedAmountTooSmallForInstallmentCountDecisionReason}, deniedResult.ReasonCodes)

	approvedResult := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 4,
		CreditAccount:        modelCreditAccount(4, 0),
		Eligibility:          approvedEligibility(),
	})
	require.Equal(t, model.ApprovedDecisionOutcome, approvedResult.Outcome)
}

func TestEvaluateFinancing_RemainderAllocatedToFirstInstallment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		financedAmountMinor  uint64
		wantFirstPrincipal   uint64
		wantRegularPrincipal uint64
	}{
		{name: "evenly divisible", financedAmountMinor: 100_000, wantFirstPrincipal: 25_000, wantRegularPrincipal: 25_000},
		{name: "remainder of 1", financedAmountMinor: 100_001, wantFirstPrincipal: 25_001, wantRegularPrincipal: 25_000},
		{name: "remainder of 3", financedAmountMinor: 100_003, wantFirstPrincipal: 25_003, wantRegularPrincipal: 25_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := v1.New().EvaluateFinancing(checkout.FinancingInput{
				RequestedAmountMinor: tt.financedAmountMinor,
				CreditAccount:        modelCreditAccount(tt.financedAmountMinor, 0),
				Eligibility:          approvedEligibility(),
			})

			require.Equal(t, tt.wantFirstPrincipal, result.FirstInstallmentPrincipalAmountMinor)
			require.Equal(t, tt.wantRegularPrincipal, result.RegularInstallmentPrincipalAmountMinor)
		})
	}
}

func TestEvaluateFinancing_DoesNotOverflowFirstInstallmentTotal(t *testing.T) {
	t.Parallel()

	result := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: uint64(math.MaxInt64),
		CreditAccount:        modelCreditAccount(uint64(math.MaxInt64/2), 0),
		Eligibility:          approvedEligibility(),
	})

	require.LessOrEqual(t, result.FirstInstallmentTotalAmountMinor, uint64(math.MaxInt64))
}

func TestEvaluateFinancing_DeterministicForIdenticalInput(t *testing.T) {
	t.Parallel()

	input := checkout.FinancingInput{
		RequestedAmountMinor: 100_000,
		CreditAccount:        modelCreditAccount(80_000, 10_000),
		Eligibility:          approvedEligibility(),
	}

	first := v1.New().EvaluateFinancing(input)
	second := v1.New().EvaluateFinancing(input)

	require.Equal(t, first, second)
}

func TestEvaluateFinancing_LimitBelowExposure_AvailableIsZero(t *testing.T) {
	t.Parallel()

	result := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: 50_000,
		CreditAccount:        modelCreditAccount(40_000, 80_000),
		Eligibility:          approvedEligibility(),
	})

	require.Equal(t, model.DeniedDecisionOutcome, result.Outcome)
	require.Equal(t, []string{model.NoAvailableCreditDecisionReason}, result.ReasonCodes)
	require.Equal(t, uint64(0), result.AvailableCreditMinor)
}
