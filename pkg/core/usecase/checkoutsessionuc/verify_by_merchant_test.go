package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestVerifyByMerchant_Success(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	terminalID := model.MerchantTerminalID(uuid.New())
	pending := h.verifyPendingSession(withContract(h.contractID))
	verifiedAt := h.now.UTC()
	verified := pending
	verified.Status = model.VerifiedByMerchantCheckoutSessionStatus
	verified.MerchantVerifiedAt = &verifiedAt
	verified.CompletedAt = &verifiedAt
	verified.MerchantVerifiedTerminalID = &terminalID

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.merchantTx.EXPECT().GetByID(h.ctx, h.merchantPID).Return(model.MerchantProfile{
		ID:                 h.merchantPID,
		ExternalMerchantID: h.merchantID,
		Status:             model.ActiveMerchantStatus,
	}, nil).Once()
	h.contractTx.EXPECT().GetByID(h.ctx, h.contractID).Return(model.Contract{
		ID:     h.contractID,
		Status: model.ActiveContractStatus,
	}, nil).Once()
	h.sessionTx.EXPECT().
		MarkVerifiedByMerchant(h.ctx, h.sessionID, terminalID, h.now).
		Return(verified, nil).Once()
	h.expectLifecycleEvent(verified, checkoutlifecycle.EventTypeVerifiedByMerchant)

	out, err := h.uc.VerifyByMerchant(h.ctx, VerifyByMerchantInput{
		CheckoutSessionToken: h.publicToken,
		ExternalMerchantID:   h.merchantID,
		MerchantOrderID:      pending.MerchantOrderID,
		TerminalID:           terminalID,
	})
	require.NoError(t, err)
	require.Equal(t, model.VerifiedByMerchantCheckoutSessionStatus, out.Status)
	require.Equal(t, pending.MerchantOrderID, out.MerchantOrderID)
	require.True(t, out.VerifiedAt.Equal(verifiedAt))
}

func TestVerifyByMerchant_IdempotentReplay(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	terminalID := model.MerchantTerminalID(uuid.New())
	verifiedAt := h.now.Add(-5 * time.Minute).UTC()
	session := h.session(
		withStatus(model.VerifiedByMerchantCheckoutSessionStatus),
		withContract(h.contractID),
		func(s *model.CheckoutSession) {
			s.MerchantVerifiedAt = &verifiedAt
			s.CompletedAt = &verifiedAt
			s.MerchantVerifiedTerminalID = &terminalID
		},
	)

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantTx.EXPECT().GetByID(h.ctx, h.merchantPID).Return(model.MerchantProfile{
		ID:                 h.merchantPID,
		ExternalMerchantID: h.merchantID,
		Status:             model.ActiveMerchantStatus,
	}, nil).Once()
	// No MarkVerifiedByMerchant / lifecycle expectations on replay.

	out, err := h.uc.VerifyByMerchant(h.ctx, VerifyByMerchantInput{
		CheckoutSessionToken: h.publicToken,
		ExternalMerchantID:   h.merchantID,
		MerchantOrderID:      session.MerchantOrderID,
		TerminalID:           terminalID,
	})
	require.NoError(t, err)
	require.Equal(t, model.VerifiedByMerchantCheckoutSessionStatus, out.Status)
	require.True(t, out.VerifiedAt.Equal(verifiedAt))
}

func TestVerifyByMerchant_RejectsAfterDeadline(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	terminalID := model.MerchantTerminalID(uuid.New())
	pendingAt := h.now.Add(-31 * time.Minute).UTC()
	deadlineAt := h.now.UTC() // now == deadline → reject
	version := "v1"
	pending := h.session(
		withStatus(model.VerifyPendingCheckoutSessionStatus),
		withContract(h.contractID),
		func(s *model.CheckoutSession) {
			s.VerificationPendingAt = &pendingAt
			s.VerificationDeadlineAt = &deadlineAt
			s.VerificationPolicyVersion = &version
		},
	)

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.merchantTx.EXPECT().GetByID(h.ctx, h.merchantPID).Return(model.MerchantProfile{
		ID:                 h.merchantPID,
		ExternalMerchantID: h.merchantID,
		Status:             model.ActiveMerchantStatus,
	}, nil).Once()
	h.contractTx.EXPECT().GetByID(h.ctx, h.contractID).Return(model.Contract{
		ID:     h.contractID,
		Status: model.ActiveContractStatus,
	}, nil).Once()

	_, err := h.uc.VerifyByMerchant(h.ctx, VerifyByMerchantInput{
		CheckoutSessionToken: h.publicToken,
		ExternalMerchantID:   h.merchantID,
		MerchantOrderID:      pending.MerchantOrderID,
		TerminalID:           terminalID,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_verification_deadline_passed")
}

func TestVerifyByMerchant_RejectsWrongOrderID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	terminalID := model.MerchantTerminalID(uuid.New())
	pending := h.verifyPendingSession(withContract(h.contractID))

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.merchantTx.EXPECT().GetByID(h.ctx, h.merchantPID).Return(model.MerchantProfile{
		ID:                 h.merchantPID,
		ExternalMerchantID: h.merchantID,
		Status:             model.ActiveMerchantStatus,
	}, nil).Once()

	_, err := h.uc.VerifyByMerchant(h.ctx, VerifyByMerchantInput{
		CheckoutSessionToken: h.publicToken,
		ExternalMerchantID:   h.merchantID,
		MerchantOrderID:      "other-order",
		TerminalID:           terminalID,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_verification_conflict")
}

func TestVerifyByMerchant_ForeignMerchantNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	terminalID := model.MerchantTerminalID(uuid.New())
	pending := h.verifyPendingSession(withContract(h.contractID))

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.merchantTx.EXPECT().GetByID(h.ctx, h.merchantPID).Return(model.MerchantProfile{
		ID:                 h.merchantPID,
		ExternalMerchantID: model.ExternalMerchantID(uuid.New()),
		Status:             model.ActiveMerchantStatus,
	}, nil).Once()

	_, err := h.uc.VerifyByMerchant(h.ctx, VerifyByMerchantInput{
		CheckoutSessionToken: h.publicToken,
		ExternalMerchantID:   h.merchantID,
		MerchantOrderID:      pending.MerchantOrderID,
		TerminalID:           terminalID,
	})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func TestVerifyByMerchant_RejectsProcessing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	terminalID := model.MerchantTerminalID(uuid.New())
	processing := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)

	h.expectPool(1, 1)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(processing, nil).Once()
	h.merchantTx.EXPECT().GetByID(h.ctx, h.merchantPID).Return(model.MerchantProfile{
		ID:                 h.merchantPID,
		ExternalMerchantID: h.merchantID,
		Status:             model.ActiveMerchantStatus,
	}, nil).Once()

	_, err := h.uc.VerifyByMerchant(h.ctx, VerifyByMerchantInput{
		CheckoutSessionToken: h.publicToken,
		ExternalMerchantID:   h.merchantID,
		MerchantOrderID:      processing.MerchantOrderID,
		TerminalID:           terminalID,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_verification_conflict")
}
