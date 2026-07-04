package otp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	otppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/otp"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

func TestRequest_IssuesAndSubmitsNotification(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	out, err := h.request()
	require.NoError(t, err)
	require.True(t, out.OTPSent)
	require.Equal(t, otp.AvailabilityAwaitingEntry, out.Availability)
	require.NotNil(t, out.OTPExpiresAt)
	require.NotNil(t, out.OTPNextIssueAt)
	require.Equal(t, 1, h.notification.count())
	require.Equal(t, testIdempotency+out.Challenge.IssueID.String(), h.notification.last().IdempotencyKey)
	require.NotContains(t, *out.Challenge.Verifier, h.notification.last().Variables["code"])
	require.Equal(t, 1, out.Challenge.SendCount)
}

func TestRequest_ActiveChallengeReplaysWithoutResending(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	first, err := h.request()
	require.NoError(t, err)
	require.True(t, first.OTPSent)
	require.Equal(t, 1, h.notification.count())

	second, err := h.request()
	require.NoError(t, err)
	require.False(t, second.OTPSent)
	require.Equal(t, 1, h.notification.count())
	require.Equal(t, *first.Challenge.IssueID, *second.Challenge.IssueID)
	require.Equal(t, otp.AvailabilityAwaitingEntry, second.Availability)
}

func TestRequest_ConsumedBlocksUntilDeadline(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	issued, err := h.request()
	require.NoError(t, err)

	code := mustCodeFromSubmit(t, h.notification)
	_, err = h.verify(code)
	require.NoError(t, err)

	_, err = h.request()
	requireMessageKey(t, err, "error.otp_issuance_window_active")
	require.Equal(t, 1, h.notification.count())

	h.advance(issued.OTPNextIssueAt.Sub(h.now))

	out, err := h.request()
	require.NoError(t, err)
	require.True(t, out.OTPSent)
	require.Equal(t, 2, h.notification.count())
	require.NotEqual(t, *issued.Challenge.IssueID, *out.Challenge.IssueID)
}

func TestRequest_FreshIssuanceExactlyAtDeadline(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	issued, err := h.request()
	require.NoError(t, err)
	code := mustCodeFromSubmit(t, h.notification)
	_, err = h.verify(code)
	require.NoError(t, err)

	deadline := *issued.OTPNextIssueAt
	h.advance(deadline.Sub(h.now))

	out, err := h.request()
	require.NoError(t, err)
	require.True(t, out.OTPSent)
}

func TestRequest_NotificationRejectedClearsIssue(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.notification.err = notification.Rejected(errors.New("provider rejected"))

	_, err := h.request()
	require.Error(t, err)
	require.True(t, notification.IsRejected(err))

	row, getErr := h.challenges.GetByPurposeSubject(h.ctx, testPurpose, testSubjectKey)
	require.NoError(t, getErr)
	require.Nil(t, row.IssueID)
	require.Nil(t, row.Verifier)
	require.Nil(t, row.NextIssueAt)
}

func TestRequest_RateLimitEnforced(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authorizer.rateLimits = []otp.RateLimitBudget{
		{
			Scope:    model.OTPRateLimitScopePhone,
			ScopeKey: "phone-fp",
			Window:   10 * time.Minute,
			MaxSends: 1,
		},
	}

	_, err := h.request()
	require.NoError(t, err)

	h.advance(2 * time.Minute)

	_, err = h.request()
	requireMessageKey(t, err, "error.otp_rate_limited")
	require.Equal(t, 1, h.notification.count())
}

func TestRequest_SendLimitEnforced(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for i := 0; i < 3; i++ {
		out, err := h.request()
		require.NoError(t, err)
		require.True(t, out.OTPSent)

		code := mustCodeFromSubmit(t, h.notification)
		_, err = h.verify(code)
		require.NoError(t, err)

		h.advance(2 * time.Minute)
	}

	_, err := h.request()
	requireMessageKey(t, err, "error.otp_send_limit_reached")
}

func TestRequest_AuthorizerErrorSurfaces(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authorizer.err = otpIssuanceBlockedCause()

	_, err := h.request()
	require.Error(t, err)
	require.Equal(t, h.authorizer.err, err)
	require.Equal(t, 0, h.notification.count())
}

func otpIssuanceBlockedCause() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.otp_issuance_not_allowed",
		errors.New("blocked"),
		cerr.Details{},
	)
}

func TestRequest_NotificationRunsWhenPostNotifyProjectionFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.uc.SetAfterNotifyProjectForTest(func(context.Context) error {
		return errors.New("projection failed")
	})

	out, err := h.request()
	require.NoError(t, err)
	require.True(t, out.OTPSent)
	require.Equal(t, 1, h.notification.count())
	require.NotNil(t, out.OTPExpiresAt)
	require.NotNil(t, out.OTPNextIssueAt)
}

func TestRequest_RateLimitsSortedByScopeThenKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// Deliberately reverse of lexicographic (Scope, ScopeKey) order so sorting
	// is observable: merchant/a before phone/z.
	h.authorizer.rateLimits = []otp.RateLimitBudget{
		{
			Scope:    model.OTPRateLimitScopePhone,
			ScopeKey: "z-phone",
			Window:   10 * time.Minute,
			MaxSends: 5,
		},
		{
			Scope:    model.OTPRateLimitScopeMerchant,
			ScopeKey: "a-merchant",
			Window:   10 * time.Minute,
			MaxSends: 5,
		},
	}

	_, err := h.request()
	require.NoError(t, err)

	h.challenges.mu.Lock()
	order := append([]rateLimitKey(nil), h.challenges.rateLimitOrder...)
	h.challenges.mu.Unlock()

	require.Equal(t, []rateLimitKey{
		{scope: model.OTPRateLimitScopeMerchant, scopeKey: "a-merchant"},
		{scope: model.OTPRateLimitScopePhone, scopeKey: "z-phone"},
	}, order)
}

func TestRequest_NotificationDetachedFromParentCancel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	unblock := make(chan struct{})
	h.notification.blockUntil = unblock

	parent, cancelParent := context.WithCancel(context.Background())
	h.ctx = parent

	done := make(chan error, 1)
	go func() {
		_, err := h.request()
		done <- err
	}()

	// Wait until Submit is blocked inside delivery, then cancel the parent.
	require.Eventually(t, func() bool {
		h.notification.mu.Lock()
		defer h.notification.mu.Unlock()

		return h.notification.sawCtx != nil
	}, time.Second, 5*time.Millisecond)

	cancelParent()
	close(unblock)

	require.NoError(t, <-done)
	require.Equal(t, 1, h.notification.count())

	row, err := h.challenges.GetByPurposeSubject(context.Background(), testPurpose, testSubjectKey)
	require.NoError(t, err)
	require.NotNil(t, row.NotificationAcceptedAt)
	require.NotNil(t, row.Verifier)
}

func TestRequest_NotificationTimeoutLeavesChallengeUsable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	unblock := make(chan struct{})
	h.notification.blockUntil = unblock

	uc, err := otp.NewWithNotificationTimeout(
		h.pool,
		h.challenges,
		h.hasher,
		h.notification,
		otppolicy.V1(),
		adapterclock.NewFixed(h.now, time.UTC),
		h.random,
		20*time.Millisecond,
	)
	require.NoError(t, err)
	h.uc = uc

	_, err = h.request()
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 0, h.notification.count())

	row, getErr := h.challenges.GetByPurposeSubject(h.ctx, testPurpose, testSubjectKey)
	require.NoError(t, getErr)
	require.NotNil(t, row.Verifier, "ambiguous timeout must not ClearIssue")
	require.Nil(t, row.NotificationAcceptedAt)

	close(unblock)
}
