package v2

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// EvaluateEligibility applies the same merchant/customer/credit prerequisites
// as other checkout policy versions. V2-specific financing behavior is kept in
// EvaluateFinancing so the policy family preserves one consistent phase model.
func (p policyV2) EvaluateEligibility(input checkout.EligibilityInput) checkout.EligibilityResult {
	result := checkout.EligibilityResult{
		Outcome:      model.DeniedDecisionOutcome,
		Jurisdiction: input.CustomerProfile.Jurisdiction,
	}

	if !input.CustomerProfile.IsActive() {
		return denyEligibility(result, model.CustomerProfileNotActiveDecisionReason)
	}

	if input.MerchantProfile.Status != model.ActiveMerchantStatus {
		return denyEligibility(result, model.MerchantProfileNotActiveDecisionReason)
	}

	if input.MerchantProfile.RequiresBranchMode && !input.BranchRequested {
		return denyEligibility(result, model.MerchantBranchProfileRequiredDecisionReason)
	}

	var branch *model.MerchantBranchProfile
	if input.BranchRequested {
		if input.MerchantBranchProfile == nil {
			return denyEligibility(result, model.MerchantBranchProfileNotActiveDecisionReason)
		}

		branch = input.MerchantBranchProfile
		if branch.MerchantID != input.MerchantProfile.MerchantID {
			return denyEligibility(result, model.MerchantBranchDoesNotBelongToMerchantDecisionReason)
		}

		if branch.Status != model.ActiveMerchantBranchStatus {
			return denyEligibility(result, model.MerchantBranchProfileNotActiveDecisionReason)
		}
	}

	currencies := input.MerchantProfile.Currencies
	maxAmountMinor := input.MerchantProfile.MaxAmountMinor
	maxAmountCurrency := input.MerchantProfile.MaxAmountCurrency
	maxTenureMonths := input.MerchantProfile.MaxTenureMonths

	if branch != nil {
		if len(branch.Currencies) > 0 {
			currencies = branch.Currencies
		}

		if branch.MaxAmountOverrideMinor != nil {
			maxAmountMinor = *branch.MaxAmountOverrideMinor
		}

		if branch.MaxAmountOverrideCurrency != nil {
			maxAmountCurrency = *branch.MaxAmountOverrideCurrency
		}

		if branch.MaxTenureMonthsOverride != nil {
			maxTenureMonths = *branch.MaxTenureMonthsOverride
		}

		if branch.Jurisdiction != "" && branch.Jurisdiction != input.CustomerProfile.Jurisdiction {
			return denyEligibility(result, model.JurisdictionMismatchDecisionReason)
		}
	}

	if input.MerchantProfile.Jurisdiction != "" &&
		input.MerchantProfile.Jurisdiction != input.CustomerProfile.Jurisdiction {
		return denyEligibility(result, model.JurisdictionMismatchDecisionReason)
	}

	if !supportsCurrency(currencies, input.Currency) {
		return denyEligibility(result, model.CurrencyNotSupportedDecisionReason)
	}

	if maxTenureMonths > 0 && uint(p.Terms().InstallmentCount) > maxTenureMonths {
		return denyEligibility(result, model.InstallmentCountNotSupportedDecisionReason)
	}

	if reasonCode, ok := checkMerchantMaxAmount(
		maxAmountMinor, maxAmountCurrency, input.Currency, input.RequestedAmountMinor,
	); !ok {
		return denyEligibility(result, reasonCode)
	}

	if !input.HasCreditAccount {
		return denyEligibility(result, model.CreditLimitMissingDecisionReason)
	}

	if input.CreditAccount.Status != model.ActiveCreditAccountStatus {
		return denyEligibility(result, model.CreditLimitNotActiveDecisionReason)
	}

	if input.CreditAccount.Currency != input.Currency {
		return denyEligibility(result, model.CurrencyNotSupportedDecisionReason)
	}

	if input.CreditAccount.ExpiresAt != nil && !input.CreditAccount.ExpiresAt.After(input.Now) {
		return denyEligibility(result, model.CreditLimitExpiredDecisionReason)
	}

	result.Outcome = model.ApprovedDecisionOutcome
	return result
}

func denyEligibility(result checkout.EligibilityResult, reasonCode string) checkout.EligibilityResult {
	result.Outcome = model.DeniedDecisionOutcome
	result.ReasonCodes = []string{reasonCode}
	return result
}

func checkMerchantMaxAmount(
	maxAmountMinor uint64,
	maxAmountCurrency model.Currency,
	currency model.Currency,
	requestedAmountMinor uint64,
) (string, bool) {
	if maxAmountMinor == 0 {
		return "", true
	}

	if maxAmountCurrency != currency {
		return model.MerchantAmountCurrencyMismatchDecisionReason, false
	}

	if requestedAmountMinor > maxAmountMinor {
		return model.RequestedAmountExceedsMerchantLimitDecisionReason, false
	}

	return "", true
}

func supportsCurrency(currencies []model.Currency, currency model.Currency) bool {
	for _, supported := range currencies {
		if supported == currency {
			return true
		}
	}

	return false
}
