package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCheckoutContractSessionMatchesAttempt(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		10,
		18,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	expires := now.Add(time.Hour)
	intent := "pi_1"
	sessionID := "ps_1"
	url := "https://pay.example/session"
	fcAttempt := "fpa_1"

	contract := model.Contract{
		Status:                    model.PaymentPendingContractStatus,
		ExpiresAt:                 expires,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
		FinancialPaymentExpiresAt: &expires,
		FinancialPaymentAttemptID: &fcAttempt,
	}
	attempt := model.PaymentAttempt{
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
		FinancialPaymentExpiresAt: &expires,
		FinancialPaymentAttemptID: &fcAttempt,
	}

	require.True(t, model.CheckoutContractSessionMatchesAttempt(contract, attempt))
	require.True(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, now))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, expires))

	// FC session later than BNPL contract deadline: effective deadline is BNPL.
	contractDeadline := now.Add(15 * time.Minute)
	fcLater := now.Add(45 * time.Minute)
	contract.ExpiresAt = contractDeadline
	contract.FinancialPaymentExpiresAt = &fcLater
	attempt.FinancialPaymentExpiresAt = &fcLater
	require.True(t, model.CheckoutContractSessionMatchesAttempt(contract, attempt))
	require.True(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, now.Add(10*time.Minute)))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, contractDeadline))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, contractDeadline.Add(time.Second)))
	require.False(t, contract.AllowsCheckoutPaymentInitiationAt(contractDeadline))
	require.True(t, contract.AllowsCheckoutPaymentInitiationAt(now))

	otherIntent := "pi_other"
	mismatched := attempt
	mismatched.FinancialPaymentIntentID = &otherIntent
	require.False(t, model.CheckoutContractSessionMatchesAttempt(contract, mismatched))

	incomplete := attempt
	incomplete.PaymentURL = nil
	require.False(t, model.PaymentAttemptHasFinancialCheckoutSession(incomplete))
	require.False(t, model.CheckoutContractSessionMatchesAttempt(contract, incomplete))
}

func TestFinancialCheckoutSessionCompleteness_RejectsZeroExpiresAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	intent := "pi_1"
	sessionID := "ps_1"
	url := "https://pay.example/session"
	zero := time.Time{}
	future := now.Add(time.Hour)
	bnplDeadline := now.Add(30 * time.Minute)

	contract := model.Contract{
		Status:                    model.PaymentPendingContractStatus,
		ExpiresAt:                 bnplDeadline,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
	}
	attempt := model.PaymentAttempt{
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
	}

	require.False(t, model.ContractHasFinancialCheckoutPaymentSession(contract))
	require.False(t, model.PaymentAttemptHasFinancialCheckoutSession(attempt))

	contract.FinancialPaymentExpiresAt = &zero
	attempt.FinancialPaymentExpiresAt = &zero
	require.False(t, model.ContractHasFinancialCheckoutPaymentSession(contract))
	require.False(t, model.PaymentAttemptHasFinancialCheckoutSession(attempt))
	require.False(t, model.CheckoutContractSessionMatchesAttempt(contract, attempt))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, now))

	contract.FinancialPaymentExpiresAt = &future
	attempt.FinancialPaymentExpiresAt = &future
	require.True(t, model.ContractHasFinancialCheckoutPaymentSession(contract))
	require.True(t, model.PaymentAttemptHasFinancialCheckoutSession(attempt))
	require.True(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, now))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, bnplDeadline))

	// Valid FC expiry later than BNPL deadline remains complete; usability uses BNPL.
	fcLater := bnplDeadline.Add(time.Hour)
	contract.FinancialPaymentExpiresAt = &fcLater
	attempt.FinancialPaymentExpiresAt = &fcLater
	require.True(t, model.CheckoutContractSessionMatchesAttempt(contract, attempt))
	require.True(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, now))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contract, attempt, bnplDeadline))
}
