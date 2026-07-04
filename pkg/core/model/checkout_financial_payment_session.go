package model

import (
	"strings"
	"time"
)

// ContractHasFinancialCheckoutPaymentSession reports whether the Contract
// carries a complete Financial Core checkout payment-session snapshot.
//
// Completeness requires a nonempty PaymentIntent ID, PaymentSession ID,
// customer payment URL, and a session expiry. It does not judge whether the
// session is still usable at a wall-clock instant.
func ContractHasFinancialCheckoutPaymentSession(contractRow Contract) bool {
	return contractRow.FinancialPaymentIntentID != nil &&
		strings.TrimSpace(*contractRow.FinancialPaymentIntentID) != "" &&
		contractRow.FinancialPaymentSessionID != nil &&
		strings.TrimSpace(*contractRow.FinancialPaymentSessionID) != "" &&
		contractRow.FinancialPaymentLink != nil &&
		strings.TrimSpace(*contractRow.FinancialPaymentLink) != "" &&
		contractRow.FinancialPaymentExpiresAt != nil
}

// PaymentAttemptHasFinancialCheckoutSession reports whether the PaymentAttempt
// carries a complete Financial Core checkout payment-session snapshot.
func PaymentAttemptHasFinancialCheckoutSession(attempt PaymentAttempt) bool {
	return attempt.FinancialPaymentIntentID != nil &&
		strings.TrimSpace(*attempt.FinancialPaymentIntentID) != "" &&
		attempt.FinancialPaymentSessionID != nil &&
		strings.TrimSpace(*attempt.FinancialPaymentSessionID) != "" &&
		attempt.PaymentURL != nil &&
		strings.TrimSpace(*attempt.PaymentURL) != "" &&
		attempt.FinancialPaymentExpiresAt != nil
}

// CheckoutContractSessionMatchesAttempt reports whether Contract and
// PaymentAttempt agree on the Financial Core checkout payment-session identity.
//
// Agreement requires both sides to be complete and to share the same
// PaymentIntent ID, PaymentSession ID, customer payment URL, and expiry.
// When both sides also store a Financial PaymentAttempt ID, those must agree.
func CheckoutContractSessionMatchesAttempt(contractRow Contract, attempt PaymentAttempt) bool {
	if !ContractHasFinancialCheckoutPaymentSession(contractRow) ||
		!PaymentAttemptHasFinancialCheckoutSession(attempt) {
		return false
	}

	if *contractRow.FinancialPaymentIntentID != *attempt.FinancialPaymentIntentID ||
		*contractRow.FinancialPaymentSessionID != *attempt.FinancialPaymentSessionID ||
		*contractRow.FinancialPaymentLink != *attempt.PaymentURL ||
		!contractRow.FinancialPaymentExpiresAt.Equal(*attempt.FinancialPaymentExpiresAt) {
		return false
	}

	if contractRow.FinancialPaymentAttemptID != nil && attempt.FinancialPaymentAttemptID != nil &&
		*contractRow.FinancialPaymentAttemptID != *attempt.FinancialPaymentAttemptID {
		return false
	}

	return true
}

// CheckoutPaymentSessionUsableAt reports whether a matched Contract/Attempt
// payment session is still within its Financial Core expiry at now.
func CheckoutPaymentSessionUsableAt(contractRow Contract, attempt PaymentAttempt, now time.Time) bool {
	if !CheckoutContractSessionMatchesAttempt(contractRow, attempt) {
		return false
	}

	return contractRow.FinancialPaymentExpiresAt.UTC().After(now.UTC())
}
