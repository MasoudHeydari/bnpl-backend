package decisionuc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// evaluateCheckoutTerms runs the shared eligibility + financing pipeline used by
// Create (persist) and RevalidateCheckoutOffer (read-only honorability check).
//
// Policy selection is the caller's responsibility: Create resolves the active
// assignment, while revalidation resolves the Decision's persisted PolicyID /
// PolicyVersion snapshot.
func evaluateCheckoutTerms(
	selectedPolicy checkout.Policy,
	now time.Time,
	requestedAmountMinor uint64,
	currency model.Currency,
	customerProfile model.CustomerProfile,
	merchantProfile model.MerchantProfile,
	branchProfile *model.MerchantBranchProfile,
	creditAccount model.CreditAccount,
	hasCreditAccount bool,
) (checkout.EligibilityResult, checkout.FinancingResult) {
	eligibility := selectedPolicy.EvaluateEligibility(checkout.EligibilityInput{
		Now:                   now,
		RequestedAmountMinor:  requestedAmountMinor,
		Currency:              currency,
		BranchRequested:       branchProfile != nil,
		CustomerProfile:       customerProfile,
		MerchantProfile:       merchantProfile,
		MerchantBranchProfile: branchProfile,
		CreditAccount:         creditAccount,
		HasCreditAccount:      hasCreditAccount,
	})

	financing := selectedPolicy.EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: requestedAmountMinor,
		CreditAccount:        creditAccount,
		Eligibility:          eligibility,
	})

	return eligibility, financing
}

// materialOfferTermsMatch reports whether a fresh financing evaluation still
// produces the same customer-facing obligations as the persisted allocation.
//
// available_credit_minor is intentionally ignored: it is an evaluation-time
// snapshot, not a customer obligation. A drop from 200 to 150 must not force
// reconfirmation when financed/upfront terms are unchanged.
func materialOfferTermsMatch(
	allocation model.CheckoutDecisionAllocation,
	financing checkout.FinancingResult,
) bool {
	if !outcomeCreatesInstallmentOffer(financing.Outcome) {
		return false
	}

	if financing.FinancedAmountMinor != allocation.ApprovedFinancedAmountMinor {
		return false
	}

	if financing.UpfrontAmountMinor != allocation.UpfrontAmountMinor {
		return false
	}

	return true
}
