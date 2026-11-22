package checkoutsessionuc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestExpirePendingMerchantVerifications_MovesDueSessions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	pendingAt := h.now.Add(-31 * time.Minute).UTC()
	deadlineAt := h.now.Add(-time.Minute).UTC()
	version := "v1"
	due := h.session(
		withStatus(model.VerifyPendingCheckoutSessionStatus),
		withContract(h.contractID),
		func(s *model.CheckoutSession) {
			s.VerificationPendingAt = &pendingAt
			s.VerificationDeadlineAt = &deadlineAt
			s.VerificationPolicyVersion = &version
		},
	)
	cancellationPendingAt := h.now.UTC()
	moved := due
	moved.Status = model.CancellationPendingCheckoutSessionStatus
	moved.CancellationPendingAt = &cancellationPendingAt

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().
		ClaimDueVerifyPending(h.ctx, h.now, 25).
		Return([]model.CheckoutSession{due}, nil).Once()
	h.sessionTx.EXPECT().
		MarkCancellationPending(h.ctx, due.ID, h.now).
		Return(moved, nil).Once()
	h.expectLifecycleEvent(moved, checkoutlifecycle.EventTypeCancellationPending)

	out, err := h.uc.ExpirePendingMerchantVerifications(h.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, out.Claimed)
	require.Equal(t, 1, out.Expired)
}

func TestExpirePendingMerchantVerifications_IgnoresConcurrentVerifyWins(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	pendingAt := h.now.Add(-31 * time.Minute).UTC()
	deadlineAt := h.now.Add(-time.Minute).UTC()
	version := "v1"
	due := h.session(
		withStatus(model.VerifyPendingCheckoutSessionStatus),
		withContract(h.contractID),
		func(s *model.CheckoutSession) {
			s.VerificationPendingAt = &pendingAt
			s.VerificationDeadlineAt = &deadlineAt
			s.VerificationPolicyVersion = &version
		},
	)

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().
		ClaimDueVerifyPending(h.ctx, h.now, 25).
		Return([]model.CheckoutSession{due}, nil).Once()
	h.sessionTx.EXPECT().
		MarkCancellationPending(h.ctx, due.ID, h.now).
		Return(model.CheckoutSession{}, cerr.Conflict(errors.New("already verified"))).Once()

	out, err := h.uc.ExpirePendingMerchantVerifications(h.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, out.Claimed)
	require.Equal(t, 0, out.Expired)
}

func TestExpirePendingMerchantVerifications_EmptyClaim(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().
		ClaimDueVerifyPending(h.ctx, h.now, 25).
		Return(nil, nil).Once()

	out, err := h.uc.ExpirePendingMerchantVerifications(h.ctx)
	require.NoError(t, err)
	require.Equal(t, 0, out.Claimed)
	require.Equal(t, 0, out.Expired)
}
