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

	otherIntent := "pi_other"
	mismatched := attempt
	mismatched.FinancialPaymentIntentID = &otherIntent
	require.False(t, model.CheckoutContractSessionMatchesAttempt(contract, mismatched))

	incomplete := attempt
	incomplete.PaymentURL = nil
	require.False(t, model.PaymentAttemptHasFinancialCheckoutSession(incomplete))
	require.False(t, model.CheckoutContractSessionMatchesAttempt(contract, incomplete))
}
