package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPaymentAttempt_EligibleForSessionCreationExpiration(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 16, 0, 0, time.UTC)
	safety := FinancialSessionCreateExpirationSafetyInterval
	sessionReason := PaymentSessionCreationTimeoutPaymentReconciliationReason
	resultTimeout := PaymentResultTimeoutPaymentReconciliationReason
	sessionID := "fc-session-1"

	base := PaymentAttempt{
		Purpose:   InstallmentRepaymentFinancialCorePaymentPurpose,
		State:     CreatedPaymentAttemptState,
		CreatedAt: now.Add(-InstallmentRepaymentPaymentSessionTTL - time.Second),
	}

	t.Run("recently created", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.CreatedAt = now.Add(-5 * time.Second)
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("before ttl boundary", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.CreatedAt = now.Add(-InstallmentRepaymentPaymentSessionTTL + time.Second)
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("just after ttl with recent third call lease", func(t *testing.T) {
		t.Parallel()
		attempt := base
		leaseUntil := now.Add(5 * time.Second)
		attempt.FinancialSessionCreateLeaseUntil = &leaseUntil
		attempt.FinancialSessionCreateAttemptCount = MaxFinancialSessionCreateAttempts
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("active lease", func(t *testing.T) {
		t.Parallel()
		attempt := base
		leaseUntil := now.Add(10 * time.Second)
		attempt.FinancialSessionCreateLeaseUntil = &leaseUntil
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("lease expired but safety interval not passed", func(t *testing.T) {
		t.Parallel()
		attempt := base
		leaseUntil := now.Add(-safety + time.Second)
		attempt.FinancialSessionCreateLeaseUntil = &leaseUntil
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("fully stale created", func(t *testing.T) {
		t.Parallel()
		attempt := base
		leaseUntil := now.Add(-safety - time.Second)
		attempt.FinancialSessionCreateLeaseUntil = &leaseUntil
		assert.True(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("already submitted session present", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.State = SubmittedPaymentAttemptState
		attempt.FinancialPaymentSessionID = &sessionID
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("created with persisted session excluded", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.FinancialPaymentSessionID = &sessionID
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("reconciliation reason mismatch", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.State = ReconciliationRequiredPaymentAttemptState
		attempt.ReconciliationReason = &resultTimeout
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("session creation reconciliation eligible", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.State = ReconciliationRequiredPaymentAttemptState
		attempt.ReconciliationReason = &sessionReason
		assert.True(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("already expired", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.State = ExpiredPaymentAttemptState
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("financial outcome resolved", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.FinancialOutcomeResolved = true
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("checkout purpose excluded", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.Purpose = CheckoutFirstInstallmentFinancialCorePaymentPurpose
		assert.False(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("exact ttl boundary not yet eligible", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.CreatedAt = now.Add(-InstallmentRepaymentPaymentSessionTTL)
		assert.True(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})

	t.Run("exact safety boundary eligible", func(t *testing.T) {
		t.Parallel()
		attempt := base
		leaseUntil := now.Add(-safety)
		attempt.FinancialSessionCreateLeaseUntil = &leaseUntil
		assert.True(t, attempt.EligibleForSessionCreationExpiration(now, safety))
	})
}

func TestPaymentAttempt_SuppressesAutomaticSessionCreationReconciliation(t *testing.T) {
	t.Parallel()

	sessionReason := PaymentSessionCreationTimeoutPaymentReconciliationReason
	unknownReason := FinancialCoreStatusStillUnknownPaymentReconciliationReason
	resultTimeout := PaymentResultTimeoutPaymentReconciliationReason
	sessionID := "fc-session"

	base := PaymentAttempt{
		Purpose:                            InstallmentRepaymentFinancialCorePaymentPurpose,
		State:                              ReconciliationRequiredPaymentAttemptState,
		ReconciliationReason:               &unknownReason,
		FinancialSessionCreateAttemptCount: MaxFinancialSessionCreateAttempts,
	}

	t.Run("exhausted session creation uncertainty", func(t *testing.T) {
		t.Parallel()
		assert.True(t, base.SuppressesAutomaticSessionCreationReconciliation())
	})

	t.Run("session creation timeout reason exhausted", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.ReconciliationReason = &sessionReason
		assert.True(t, attempt.SuppressesAutomaticSessionCreationReconciliation())
	})

	t.Run("attempt count no longer gates suppression", func(t *testing.T) {
		t.Parallel()
		for _, count := range []int{0, 1, 2, 3, 4} {
			attempt := base
			attempt.FinancialSessionCreateAttemptCount = count
			assert.True(t, attempt.SuppressesAutomaticSessionCreationReconciliation(), "count=%d", count)
		}
	})

	t.Run("submitted payment outcome uncertainty unaffected", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.ReconciliationReason = &resultTimeout
		assert.False(t, attempt.SuppressesAutomaticSessionCreationReconciliation())
	})

	t.Run("partial session identity must not trigger automatic rediscovery", func(t *testing.T) {
		t.Parallel()
		attempt := base
		attempt.FinancialPaymentSessionID = &sessionID
		assert.True(t, attempt.SuppressesAutomaticSessionCreationReconciliation())
	})
}
