package contractuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestValidateConcurrentCheckoutPaymentSession_ComparesAuthoritativeFCExpiry(t *testing.T) {
	t.Parallel()

	bnplDeadline := time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC)
	storedFCExpiry := time.Date(2026, 10, 20, 12, 45, 0, 0, time.UTC)
	invoice := "inv-1"
	intent := "pi-1"
	fcAttempt := "fpa-1"
	sessionID := "ps-1"
	link := "https://pay.example/session"

	contractRow := model.Contract{
		ID:                        model.ContractID{},
		ExpiresAt:                 bnplDeadline,
		FinancialInvoiceID:        &invoice,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentAttemptID: &fcAttempt,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &link,
		FinancialPaymentExpiresAt: &storedFCExpiry,
	}

	matching := extcheckoutpayment.CreateCheckoutPaymentResult{
		InvoiceID:        invoice,
		PaymentIntentID:  intent,
		PaymentAttemptID: fcAttempt,
		PaymentSessionID: sessionID,
		PaymentLink:      link,
		ExpiresAt:        storedFCExpiry,
	}

	require.NoError(t, validateConcurrentCheckoutPaymentSession(contractRow, matching, storedFCExpiry))

	// Customer-visible effective expiry is 12:15 for both 12:45 and 13:30 FC values,
	// but authoritative FC expirations differ and must conflict.
	laterFC := storedFCExpiry.Add(45 * time.Minute)
	conflictExpiry := matching
	conflictExpiry.ExpiresAt = laterFC
	require.Error(t, validateConcurrentCheckoutPaymentSession(contractRow, conflictExpiry, laterFC))

	loc := time.FixedZone("UTC+3:30", 3*3600+30*60)
	sameInstant := matching
	sameInstant.ExpiresAt = storedFCExpiry.In(loc)
	require.NoError(t, validateConcurrentCheckoutPaymentSession(contractRow, sameInstant, storedFCExpiry.In(loc)))

	otherIntent := matching
	otherIntent.PaymentIntentID = "pi-other"
	require.Error(t, validateConcurrentCheckoutPaymentSession(contractRow, otherIntent, storedFCExpiry))

	otherSession := matching
	otherSession.PaymentSessionID = "ps-other"
	require.Error(t, validateConcurrentCheckoutPaymentSession(contractRow, otherSession, storedFCExpiry))

	otherURL := matching
	otherURL.PaymentLink = "https://pay.example/other"
	require.Error(t, validateConcurrentCheckoutPaymentSession(contractRow, otherURL, storedFCExpiry))

	attempt := model.PaymentAttempt{
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
	}
	require.NoError(t, validateConcurrentCheckoutPaymentSession(contractRow, matching, storedFCExpiry, attempt))

	wrongAttemptIntent := "pi-mismatch"
	mismatchedAttempt := model.PaymentAttempt{
		FinancialPaymentIntentID:  &wrongAttemptIntent,
		FinancialPaymentSessionID: &sessionID,
	}
	require.Error(t, validateConcurrentCheckoutPaymentSession(contractRow, matching, storedFCExpiry, mismatchedAttempt))

	// Presentation helpers remain customer-visible; durable FC expiry stays unchanged.
	require.Equal(t, bnplDeadline.UTC().Format(time.RFC3339), checkoutPaymentOutputFromContract(contractRow).ExpiresAt)
	require.True(t, contractRow.FinancialPaymentExpiresAt.UTC().Equal(storedFCExpiry))
	require.Equal(
		t,
		bnplDeadline.UTC().Format(time.RFC3339),
		checkoutPaymentOutputFromResult(matching, storedFCExpiry, bnplDeadline).ExpiresAt,
	)
}
