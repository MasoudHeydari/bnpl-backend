package v1

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// EvaluateFinancing computes available credit, the financed/upfront split,
// and installment principal allocation for an eligibility-approved checkout
// request.
//
// AvailableCreditMinor is always populated from the customer's limit and active
// exposure when computable, including when eligibility already denied the
// request (for example merchant max-amount). Denial of financing must not erase
// that snapshot — available credit is informational and is not reserved by the
// decision.
func (p policyV1) EvaluateFinancing(input checkout.FinancingInput) checkout.FinancingResult {
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

	// Eligibility already rejected the request (merchant limit, status, etc.).
	// Keep that outcome/reason and still return the available-credit snapshot.
	if input.Eligibility.Outcome == model.DeniedDecisionOutcome {
		return result
	}

	if availableCredit == 0 {
		return denyFinancing(result, model.NoAvailableCreditDecisionReason)
	}

	terms := p.Terms()
	financedAmount := input.RequestedAmountMinor
	if financedAmount > availableCredit {
		financedAmount = availableCredit
	}

	if financedAmount > terms.MaxFinancedAmountMinor {
		financedAmount = terms.MaxFinancedAmountMinor
	}

	if financedAmount == 0 {
		return denyFinancing(result, model.NoAvailableCreditDecisionReason)
	}

	upfrontAmount, err := subUint64(input.RequestedAmountMinor, financedAmount)
	if err != nil {
		return denyFinancing(result, model.RequestedAmountExceedsSupportedRangeDecisionReason)
	}

	installmentCount := terms.InstallmentCount
	if installmentCount <= 0 {
		return denyFinancing(result, model.InstallmentCountNotSupportedDecisionReason)
	}

	installmentCountUint := uint64(installmentCount)
	if financedAmount < installmentCountUint {
		return denyFinancing(result, model.FinancedAmountTooSmallForInstallmentCountDecisionReason)
	}

	basePrincipal, remainder := splitPrincipal(financedAmount, installmentCount)
	firstInstallmentPrincipal := basePrincipal + remainder
	regularInstallmentPrincipal := basePrincipal
	firstInstallmentUpfront := upfrontAmount
	firstInstallmentTotal, err := addUint64(upfrontAmount, firstInstallmentPrincipal)
	if err != nil {
		return denyFinancing(result, model.RequestedAmountExceedsSupportedRangeDecisionReason)
	}

	result.FinancedAmountMinor = financedAmount
	result.UpfrontAmountMinor = upfrontAmount
	result.RegularInstallmentPrincipalAmountMinor = regularInstallmentPrincipal
	result.FirstInstallmentPrincipalAmountMinor = firstInstallmentPrincipal
	result.FirstInstallmentUpfrontAmountMinor = firstInstallmentUpfront
	result.FirstInstallmentTotalAmountMinor = firstInstallmentTotal
	result.RegularInstallmentAmountMinor = regularInstallmentPrincipal
	result.FirstInstallmentAmountMinor = firstInstallmentTotal

	if upfrontAmount > 0 {
		result.Outcome = model.ConditionalDecisionOutcome
		result.ReasonCodes = []string{model.PartialApprovalRequiresFirstInstallmentPayDecisionReason}

		return result
	}

	result.Outcome = model.ApprovedDecisionOutcome
	result.ReasonCodes = []string{model.ApprovedFullFinancingDecisionReason}

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
	// Intentionally preserve AvailableCreditMinor. Denial reasons such as
	// financed-amount-too-small must not report the customer as having zero
	// available credit.

	return result
}

// splitPrincipal is the single implementation of the equal-split-remainder-
// to-first-installment allocation math shared by EvaluateFinancing (decision
// time) and BuildInstallmentSchedule (offer time). Both callers already
// guard against a non-positive installmentCount before calling this
// function; the guard is repeated here so the int-to-uint64 conversion is
// never unbounded from this function's own local perspective.
func splitPrincipal(financedAmountMinor uint64, installmentCount int) (basePrincipal, remainder uint64) {
	if installmentCount <= 0 {
		return 0, 0
	}

	installmentCountUint := uint64(installmentCount)

	return financedAmountMinor / installmentCountUint, financedAmountMinor % installmentCountUint
}

// availableCreditForCheckoutInquiry returns the customer's currently
// available credit for a checkout inquiry.
//
// Checkout decisions do not reserve credit. The CreditAccount reports only
// committed exposure; a separate payment_pending Contract is the concurrency
// gate that prevents two payment flows from consuming the same capacity.
func availableCreditForCheckoutInquiry(account model.CreditAccount) (uint64, error) {
	if account.LimitAmountMinor < account.ExposureAmountMinor {
		return 0, nil
	}

	return subUint64(account.LimitAmountMinor, account.ExposureAmountMinor)
}

func addUint64(a, b uint64) (uint64, error) {
	const op = "v1.addUint64"

	sum, err := model.AddAmountMinor(a, b)
	if err != nil {
		return 0, fmt.Errorf("%s: add amount minor: %w", op, err)
	}

	return sum, nil
}

func subUint64(a, b uint64) (uint64, error) {
	const op = "v1.subUint64"

	diff, err := model.SubtractAmountMinor(a, b)
	if err != nil {
		return 0, fmt.Errorf("%s: subtract amount minor: %w", op, err)
	}

	return diff, nil
}
