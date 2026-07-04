package contractuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestEnsureCheckoutPaymentSession_RejectsExpiredMatchedLink(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		10,
		20,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	expires := now
	intent := "pi_1"
	sessionID := "ps_1"
	link := "https://pay.example/expired"

	contractRow := model.Contract{
		Status:                    model.PaymentPendingContractStatus,
		ExpiresAt:                 expires,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &link,
		FinancialPaymentExpiresAt: &expires,
	}
	attempt := model.PaymentAttempt{
		State:                     model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &link,
		FinancialPaymentExpiresAt: &expires,
	}

	require.True(t, model.CheckoutContractSessionMatchesAttempt(contractRow, attempt))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contractRow, attempt, now))
	require.False(t, model.CheckoutPaymentSessionUsableAt(contractRow, attempt, now.Add(time.Nanosecond)))
	require.True(t, model.CheckoutPaymentSessionUsableAt(contractRow, attempt, now.Add(-time.Nanosecond)))

	clk := adapterclock.NewFixed(now, time.UTC)
	uc := &UseCase{clock: &clk}
	_, _, _, err := uc.ensureCheckoutPaymentSession(t.Context(), contractRow, attempt)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.checkout_payment_session_expired"), appErr.MessageKey)
}

func TestEnsureActiveCheckoutPaymentAttempt_RejectsTerminalContract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	clk := adapterclock.NewFixed(now, time.UTC)
	uc := &UseCase{clock: &clk}
	_, err := uc.ensureActiveCheckoutPaymentAttempt(
		t.Context(),
		model.Contract{Status: model.ExpiredContractStatus, ExpiresAt: now.Add(time.Hour)},
		model.PaymentAttempt{State: model.FailedPaymentAttemptState},
	)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestEnsureCheckoutPaymentSession_RejectsWhenBNPLDeadlinePassedBeforeFCExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 20, 0, 0, time.UTC)
	bnplDeadline := time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC)
	fcExpiry := time.Date(2026, 10, 20, 12, 45, 0, 0, time.UTC)
	intent := "pi_1"
	sessionID := "ps_1"
	link := "https://pay.example/still-valid-at-fc"

	contractRow := model.Contract{
		Status:                    model.PaymentPendingContractStatus,
		ExpiresAt:                 bnplDeadline,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &link,
		FinancialPaymentExpiresAt: &fcExpiry,
	}
	attempt := model.PaymentAttempt{
		State:                     model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &link,
		FinancialPaymentExpiresAt: &fcExpiry,
	}

	require.Equal(t, bnplDeadline, contractRow.EffectivePaymentDeadline())
	require.False(t, model.CheckoutPaymentSessionUsableAt(contractRow, attempt, now))
	require.False(t, contractRow.AllowsCheckoutPaymentInitiationAt(now))

	clk := adapterclock.NewFixed(now, time.UTC)
	uc := &UseCase{clock: &clk}
	_, _, _, err := uc.ensureCheckoutPaymentSession(t.Context(), contractRow, attempt)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestCheckoutPaymentOutput_CustomerVisibleExpiresAtUsesEffectiveDeadline(t *testing.T) {
	t.Parallel()

	bnplDeadline := time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC)
	fcExpiry := time.Date(2026, 10, 20, 12, 45, 0, 0, time.UTC)
	link := "https://pay.example/x"
	intent := "pi_1"
	sessionID := "ps_1"

	contractRow := model.Contract{
		ExpiresAt:                 bnplDeadline,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &link,
		FinancialPaymentExpiresAt: &fcExpiry,
	}

	fromContract := checkoutPaymentOutputFromContract(contractRow)
	require.Equal(t, bnplDeadline.UTC().Format(time.RFC3339), fromContract.ExpiresAt)
	require.True(t, contractRow.FinancialPaymentExpiresAt.UTC().Equal(fcExpiry))

	result := extcheckoutpayment.CreateCheckoutPaymentResult{
		PaymentIntentID:  intent,
		PaymentSessionID: sessionID,
		PaymentLink:      link,
		ExpiresAt:        fcExpiry,
	}
	fromResult := checkoutPaymentOutputFromResult(result, fcExpiry, bnplDeadline)
	require.Equal(t, bnplDeadline.UTC().Format(time.RFC3339), fromResult.ExpiresAt)
}
