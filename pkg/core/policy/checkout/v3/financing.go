package v3

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// EvaluateFinancing requires the whole purchase to fit monthly capacity.
// Unlike installment credit, this product never creates an upfront remainder.
func (p policyV3) EvaluateFinancing(input checkout.FinancingInput) checkout.FinancingResult {
	result := checkout.FinancingResult{
		Outcome:      input.Eligibility.Outcome,
		ReasonCodes:  append([]string(nil), input.Eligibility.ReasonCodes...),
		Jurisdiction: input.Eligibility.Jurisdiction,
	}
	if input.CreditAccount.LimitAmountMinor < input.CreditAccount.ExposureAmountMinor {
		return denyFinancing(result, model.NoAvailableCreditDecisionReason)
	}
	result.AvailableCreditMinor = input.CreditAccount.AvailableAmountMinor()
	if result.Outcome == model.DeniedDecisionOutcome {
		return result
	}
	if input.RequestedAmountMinor == 0 || input.RequestedAmountMinor > p.Terms().MaxFinancedAmountMinor {
		return denyFinancing(result, model.RequestedAmountExceedsSupportedRangeDecisionReason)
	}
	if result.AvailableCreditMinor < input.RequestedAmountMinor {
		return denyFinancing(result, model.NoAvailableCreditDecisionReason)
	}
	result.Outcome = model.ApprovedDecisionOutcome
	result.ReasonCodes = []string{model.ApprovedFullFinancingDecisionReason}
	result.FinancedAmountMinor = input.RequestedAmountMinor
	result.RegularInstallmentPrincipalAmountMinor = input.RequestedAmountMinor
	result.FirstInstallmentPrincipalAmountMinor = input.RequestedAmountMinor
	result.RegularInstallmentAmountMinor = input.RequestedAmountMinor
	result.FirstInstallmentAmountMinor = input.RequestedAmountMinor
	result.FirstInstallmentTotalAmountMinor = input.RequestedAmountMinor
	result.TotalPayableAmountMinor = input.RequestedAmountMinor
	return result
}

func denyFinancing(result checkout.FinancingResult, reason string) checkout.FinancingResult {
	result.Outcome = model.DeniedDecisionOutcome
	result.ReasonCodes = []string{reason}
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
