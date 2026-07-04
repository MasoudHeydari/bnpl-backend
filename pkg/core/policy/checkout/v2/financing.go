package v2

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// EvaluateFinancing implements v2 full-financing semantics. A checkout is
// approved only when the entire requested amount fits both customer available
// credit and the policy-owned per-contract financing ceiling. Upfront is always zero.
func (p policyV2) EvaluateFinancing(input checkout.FinancingInput) checkout.FinancingResult {
	result := checkout.FinancingResult{
		Outcome:      input.Eligibility.Outcome,
		ReasonCodes:  append([]string(nil), input.Eligibility.ReasonCodes...),
		Jurisdiction: input.Eligibility.Jurisdiction,
	}

	availableCredit, err := availableCreditForCheckoutInquiry(input.CreditAccount)
	if err != nil {
		if input.Eligibility.Outcome == model.DeniedDecisionOutcome {
			return result
		}

		return denyFinancing(result, model.NoAvailableCreditDecisionReason)
	}

	result.AvailableCreditMinor = availableCredit

	if input.Eligibility.Outcome == model.DeniedDecisionOutcome {
		return result
	}

	terms := p.Terms()
	if input.RequestedAmountMinor == 0 || input.RequestedAmountMinor > terms.MaxFinancedAmountMinor {
		return denyFinancing(result, model.RequestedAmountExceedsSupportedRangeDecisionReason)
	}

	if availableCredit < input.RequestedAmountMinor {
		return denyFinancing(result, model.NoAvailableCreditDecisionReason)
	}

	installmentCount := terms.InstallmentCount
	if installmentCount <= 0 {
		return denyFinancing(result, model.InstallmentCountNotSupportedDecisionReason)
	}

	if input.RequestedAmountMinor < uint64(installmentCount) {
		return denyFinancing(result, model.FinancedAmountTooSmallForInstallmentCountDecisionReason)
	}

	basePrincipal, remainder := splitPrincipal(input.RequestedAmountMinor, installmentCount)
	firstPrincipal := basePrincipal + remainder

	result.Outcome = model.ApprovedDecisionOutcome
	result.ReasonCodes = []string{model.ApprovedFullFinancingDecisionReason}
	result.FinancedAmountMinor = input.RequestedAmountMinor
	result.UpfrontAmountMinor = 0
	result.RegularInstallmentPrincipalAmountMinor = basePrincipal
	result.FirstInstallmentPrincipalAmountMinor = firstPrincipal
	result.FirstInstallmentUpfrontAmountMinor = 0
	result.RegularInstallmentAmountMinor = basePrincipal
	result.FirstInstallmentAmountMinor = firstPrincipal
	result.FirstInstallmentTotalAmountMinor = firstPrincipal
	result.TotalPayableAmountMinor = input.RequestedAmountMinor

	return result
}

func denyFinancing(result checkout.FinancingResult, reasonCode string) checkout.FinancingResult {
	result.Outcome = model.DeniedDecisionOutcome
	result.ReasonCodes = []string{reasonCode}
	result.FinancedAmountMinor = 0
	result.UpfrontAmountMinor = 0
	result.RegularInstallmentAmountMinor = 0
	result.FirstInstallmentAmountMinor = 0
	result.RegularInstallmentPrincipalAmountMinor = 0
	result.FirstInstallmentPrincipalAmountMinor = 0
	result.FirstInstallmentUpfrontAmountMinor = 0
	result.FirstInstallmentTotalAmountMinor = 0
	result.TotalFeeAmountMinor = 0
	result.TotalPayableAmountMinor = 0
	return result
}

func splitPrincipal(financedAmountMinor uint64, installmentCount int) (basePrincipal, remainder uint64) {
	if installmentCount <= 0 {
		return 0, 0
	}

	count := uint64(installmentCount)
	return financedAmountMinor / count, financedAmountMinor % count
}

func availableCreditForCheckoutInquiry(account model.CreditAccount) (uint64, error) {
	const op = "v2.availableCreditForCheckoutInquiry"

	if account.LimitAmountMinor < account.ExposureAmountMinor {
		return 0, nil
	}

	diff, err := model.SubtractAmountMinor(account.LimitAmountMinor, account.ExposureAmountMinor)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return diff, nil
}
