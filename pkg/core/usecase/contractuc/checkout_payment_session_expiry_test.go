package contractuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
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

	uc := &UseCase{}
	_, err := uc.ensureActiveCheckoutPaymentAttempt(
		t.Context(),
		model.Contract{Status: model.ExpiredContractStatus},
		model.PaymentAttempt{State: model.FailedPaymentAttemptState},
	)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}
