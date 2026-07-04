package financialcorepaymentreconciliationuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func testReconciliationUC() *UseCase {
	return &UseCase{
		fingerprintHasher: sha256fingerprint.New(),
		logger:            lognoop.New(),
	}
}

func ptrString(v string) *string { return &v }

func TestMergePendingSession(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	urlA := "https://pay.example/A"
	urlB := "https://pay.example/B"
	expires := now.Add(15 * time.Minute)

	t.Run("complete event data", func(t *testing.T) {
		t.Parallel()

		merged := mergePendingSession(model.PaymentAttempt{}, installmentpaymentreconciliation.ResultEventData{
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &urlA,
			ExpiresAt:                 &expires,
		})
		require.False(t, merged.IdentityConflict)
		require.True(t, merged.Complete)
		require.Equal(t, "pi-1", merged.IntentID)
		require.Equal(t, "ps-1", merged.SessionID)
		require.Equal(t, urlA, *merged.PaymentURL)
		require.True(t, merged.ExpiresAt.UTC().Equal(expires.UTC()))
	})

	t.Run("merges stored session fields with partial event", func(t *testing.T) {
		t.Parallel()

		attempt := model.PaymentAttempt{
			FinancialPaymentIntentID:  ptrString("pi-stored"),
			FinancialPaymentSessionID: ptrString("ps-stored"),
			PaymentURL:                &urlA,
			FinancialPaymentExpiresAt: &expires,
		}
		merged := mergePendingSession(attempt, installmentpaymentreconciliation.ResultEventData{})
		require.False(t, merged.IdentityConflict)
		require.True(t, merged.Complete)
		require.Equal(t, "pi-stored", merged.IntentID)
		require.Equal(t, "ps-stored", merged.SessionID)
		require.Equal(t, urlA, *merged.PaymentURL)
	})

	t.Run("identity conflict on intent", func(t *testing.T) {
		t.Parallel()

		attempt := model.PaymentAttempt{
			FinancialPaymentIntentID: ptrString("pi-stored"),
		}
		merged := mergePendingSession(attempt, installmentpaymentreconciliation.ResultEventData{
			FinancialPaymentIntentID:  ptrString("pi-other"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &urlA,
			ExpiresAt:                 &expires,
		})
		require.True(t, merged.IdentityConflict)
		require.False(t, merged.Complete)
	})

	t.Run("incomplete without payment url", func(t *testing.T) {
		t.Parallel()

		merged := mergePendingSession(model.PaymentAttempt{}, installmentpaymentreconciliation.ResultEventData{
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			ExpiresAt:                 &expires,
		})
		require.False(t, merged.IdentityConflict)
		require.False(t, merged.Complete)
	})

	t.Run("event url overrides stored", func(t *testing.T) {
		t.Parallel()

		attempt := model.PaymentAttempt{
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &urlA,
			FinancialPaymentExpiresAt: &expires,
		}
		merged := mergePendingSession(attempt, installmentpaymentreconciliation.ResultEventData{
			PaymentURL: &urlB,
		})
		require.True(t, merged.Complete)
		require.Equal(t, urlB, *merged.PaymentURL)
	})
}

func TestReconciliationResultPayloadFingerprint_DetectsActionableFieldChanges(t *testing.T) {
	t.Parallel()

	uc := testReconciliationUC()
	attemptID := model.PaymentAttemptID(uuid.New())
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	urlA := "https://pay.example/A"
	urlB := "https://pay.example/B"
	expiresA := now.Add(15 * time.Minute)
	expiresB := now.Add(30 * time.Minute)
	paidAt := now.Add(time.Minute)
	failure := "provider_failed"

	base := ProcessInput{
		EventID: "event-123",
		Data: installmentpaymentreconciliation.ResultEventData{
			RequestEventID:            installmentpaymentreconciliation.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:      attemptID,
			ReconciliationGeneration:  1,
			Status:                    installmentpaymentreconciliation.PendingResultStatus,
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentAttemptID: ptrString("fa-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &urlA,
			ExpiresAt:                 &expiresA,
			AmountMinor:               1000,
			Currency:                  "IRR",
			OccurredAt:                now,
		},
	}

	fp1, err := uc.reconciliationResultPayloadFingerprint(base)
	require.NoError(t, err)

	fpSame, err := uc.reconciliationResultPayloadFingerprint(base)
	require.NoError(t, err)
	require.Equal(t, fp1, fpSame)

	changedURL := base
	changedURL.Data.PaymentURL = &urlB
	fpURL, err := uc.reconciliationResultPayloadFingerprint(changedURL)
	require.NoError(t, err)
	require.NotEqual(t, fp1, fpURL)

	changedExpiry := base
	changedExpiry.Data.ExpiresAt = &expiresB
	fpExpiry, err := uc.reconciliationResultPayloadFingerprint(changedExpiry)
	require.NoError(t, err)
	require.NotEqual(t, fp1, fpExpiry)

	changedAttempt := base
	changedAttempt.Data.FinancialPaymentAttemptID = ptrString("fa-2")
	fpAttempt, err := uc.reconciliationResultPayloadFingerprint(changedAttempt)
	require.NoError(t, err)
	require.NotEqual(t, fp1, fpAttempt)

	changedRequest := base
	changedRequest.Data.RequestEventID = installmentpaymentreconciliation.RequestEventID(attemptID, 2)
	fpRequest, err := uc.reconciliationResultPayloadFingerprint(changedRequest)
	require.NoError(t, err)
	require.NotEqual(t, fp1, fpRequest)

	changedPaid := base
	changedPaid.Data.PaidAt = &paidAt
	fpPaid, err := uc.reconciliationResultPayloadFingerprint(changedPaid)
	require.NoError(t, err)
	require.NotEqual(t, fp1, fpPaid)

	changedFailure := base
	changedFailure.Data.FailureCode = &failure
	fpFailure, err := uc.reconciliationResultPayloadFingerprint(changedFailure)
	require.NoError(t, err)
	require.NotEqual(t, fp1, fpFailure)

	changedOccurred := base
	changedOccurred.Data.OccurredAt = now.Add(time.Second)
	fpOccurred, err := uc.reconciliationResultPayloadFingerprint(changedOccurred)
	require.NoError(t, err)
	require.NotEqual(t, fp1, fpOccurred)
}

func TestPendingResultEconomicsConflict(t *testing.T) {
	t.Parallel()

	attempt := model.PaymentAttempt{
		AmountMinor: 1000,
		Currency:    model.IRRCurrency,
	}

	require.NoError(t, pendingResultEconomicsConflict(ProcessInput{
		Data: installmentpaymentreconciliation.ResultEventData{
			AmountMinor: 1000,
			Currency:    "IRR",
		},
	}, attempt))

	require.Error(t, pendingResultEconomicsConflict(ProcessInput{
		Data: installmentpaymentreconciliation.ResultEventData{
			AmountMinor: 999,
			Currency:    "IRR",
		},
	}, attempt))

	require.Error(t, pendingResultEconomicsConflict(ProcessInput{
		Data: installmentpaymentreconciliation.ResultEventData{
			AmountMinor: 1000,
			Currency:    "USD",
		},
	}, attempt))
}

func TestRequestedInstallmentPaymentExpiresAt(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	got, ok := requestedInstallmentPaymentExpiresAt(model.PaymentAttempt{CreatedAt: created})
	require.True(t, ok)
	require.Equal(t, created.Add(installmentPaymentSessionRequestTTL), got)

	_, ok = requestedInstallmentPaymentExpiresAt(model.PaymentAttempt{})
	require.False(t, ok)
}
