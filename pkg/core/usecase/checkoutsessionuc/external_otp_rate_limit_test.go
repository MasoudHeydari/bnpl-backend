package checkoutsessionuc

import (
	"errors"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestIssueOTP_PhoneSucceedsMerchantRejectsRollsBack(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	linked := h.session()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(linked, nil).Once()
	h.sessionTx.EXPECT().
		ConsumeOTPRateLimit(h.ctx, model.CheckoutOTPRateLimitScopePhone, testifymock.AnythingOfType("string"), 10*time.Minute, 5, h.now).
		Return(nil).Once()
	h.sessionTx.EXPECT().
		ConsumeOTPRateLimit(h.ctx, model.CheckoutOTPRateLimitScopeMerchant, h.merchantPID.String(), 10*time.Minute, 30_000, h.now).
		Return(cerr.NewPublic(
			cerr.CodeTooManyRequests,
			"error.checkout_otp_rate_limited",
			errors.New("merchant bucket full"),
			cerr.Details{},
		)).Once()

	_, err := h.uc.issueOTP(h.ctx, h.publicToken, 1)
	requireCerr(t, err, cerr.CodeTooManyRequests, "error.checkout_otp_rate_limited")
}

func TestIssueOTP_PhoneRejectsDoesNotConsumeMerchant(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	linked := h.session()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(linked, nil).Once()
	h.sessionTx.EXPECT().
		ConsumeOTPRateLimit(h.ctx, model.CheckoutOTPRateLimitScopePhone, testifymock.AnythingOfType("string"), 10*time.Minute, 5, h.now).
		Return(cerr.NewPublic(
			cerr.CodeTooManyRequests,
			"error.checkout_otp_rate_limited",
			errors.New("phone bucket full"),
			cerr.Details{},
		)).Once()

	_, err := h.uc.issueOTP(h.ctx, h.publicToken, 1)
	requireCerr(t, err, cerr.CodeTooManyRequests, "error.checkout_otp_rate_limited")
}

func TestIssueOTP_BothLimitersSucceedIssueOTPFailsPropagates(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	linked := h.session()
	issueErr := errors.New("issue otp failed")
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(linked, nil).Once()
	h.expectOTPRateLimits()
	h.sessionTx.EXPECT().
		IssueOTP(h.ctx, h.sessionID, testifymock.Anything, testifymock.Anything, 1, h.now.Add(2*time.Minute), h.now).
		Return(model.CheckoutSession{}, issueErr).Once()

	_, err := h.uc.issueOTP(h.ctx, h.publicToken, 1)
	require.ErrorIs(t, err, issueErr)
}

func TestIssueOTP_ActiveOTPSkipsRateLimits(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	inFlight := h.session(withOTPVerifier(
		t,
		h.publicToken,
		testOTPCode,
		withOTPExpiresAt(h.now.Add(time.Minute)),
	))
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(inFlight, nil).Once()

	got, err := h.uc.issueOTP(h.ctx, h.publicToken, 1)
	require.NoError(t, err)
	require.False(t, got.issued)
	require.Equal(t, inFlight.ID, got.session.ID)
}

func TestStartExternal_MerchantDisplayNameFailureBeforeOTPBudget(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin, project offer, merchant display — issueOTP never opens a TX.
	h.expectPool(2, 1)

	prepared := h.session()
	h.expectProjectApprovedOffer(prepared)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(prepared, nil).Once()

	h.merchantCQ.EXPECT().GetByID(h.ctx, prepared.MerchantProfileID).Return(
		model.MerchantProfile{},
		errors.New("merchant profile unavailable"),
	).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.Error(t, err)
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_MerchantDisplayNameEmptyBeforeOTPBudget(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 1)

	prepared := h.session()
	h.expectProjectApprovedOffer(prepared)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(prepared, nil).Once()

	h.merchantCQ.EXPECT().GetByID(h.ctx, prepared.MerchantProfileID).Return(model.MerchantProfile{
		ID:          prepared.MerchantProfileID,
		DisplayName: "",
	}, nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.Empty(t, h.notificationCalls)
}
