package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestGetPartnerStatus_CompletedIncludesCommercialFields(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	completedAt := h.now.Add(-time.Minute)
	session := h.session(
		withStatus(model.CompletedCheckoutSessionStatus),
		func(s *model.CheckoutSession) {
			s.RequestedAmountMinor = 120_000_000
			s.Currency = model.Currency("IRR")
			s.CompletedAt = &completedAt
			s.MerchantOrderID = "DAMZ-ORDER-1001"
		},
	)

	h.expectPool(1, 0)
	h.merchantCQ.EXPECT().GetByMerchantID(h.ctx, h.merchantID).Return(model.MerchantProfile{
		ID:         session.MerchantProfileID,
		MerchantID: h.merchantID,
		Status:     model.ActiveMerchantStatus,
	}, nil).Once()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	output, err := h.uc.GetPartnerStatus(h.ctx, GetPartnerStatusInput{
		CheckoutSessionToken: h.publicToken,
		MerchantID:           h.merchantID,
	})
	require.NoError(t, err)
	require.Equal(t, h.publicToken, output.CheckoutSessionToken)
	require.Equal(t, "DAMZ-ORDER-1001", output.MerchantOrderID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, output.Status)
	require.EqualValues(t, 120_000_000, output.AmountMinor)
	require.Equal(t, model.Currency("IRR"), output.Currency)
	require.NotNil(t, output.CompletedAt)
	require.True(t, output.CompletedAt.Equal(completedAt))
}

func TestGetPartnerStatus_OpenPastExpiryProjectsExpiredWithoutWrite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	session := h.session(
		withStatus(model.OpenCheckoutSessionStatus),
		func(s *model.CheckoutSession) {
			s.ExpiresAt = h.now.Add(-time.Second)
			s.RequestedAmountMinor = 50_000
			s.Currency = model.Currency("IRR")
		},
	)

	h.expectPool(1, 0)
	h.merchantCQ.EXPECT().GetByMerchantID(h.ctx, h.merchantID).Return(model.MerchantProfile{
		ID:         session.MerchantProfileID,
		MerchantID: h.merchantID,
		Status:     model.ActiveMerchantStatus,
	}, nil).Once()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	// No UpdateStatus / Enqueue expectations: GET must stay read-only.

	output, err := h.uc.GetPartnerStatus(h.ctx, GetPartnerStatusInput{
		CheckoutSessionToken: h.publicToken,
		MerchantID:           h.merchantID,
	})
	require.NoError(t, err)
	require.Equal(t, model.ExpiredCheckoutSessionStatus, output.Status)
	require.Nil(t, output.CompletedAt)
	require.EqualValues(t, 50_000, output.AmountMinor)
}

func TestGetPartnerStatus_ForeignMerchantNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	session := h.session()
	session.MerchantProfileID = model.MerchantProfileID(uuid.New())

	h.expectPool(1, 0)
	h.merchantCQ.EXPECT().GetByMerchantID(h.ctx, h.merchantID).Return(model.MerchantProfile{
		ID:         h.merchantPID,
		MerchantID: h.merchantID,
		Status:     model.ActiveMerchantStatus,
	}, nil).Once()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	_, err := h.uc.GetPartnerStatus(h.ctx, GetPartnerStatusInput{
		CheckoutSessionToken: h.publicToken,
		MerchantID:           h.merchantID,
	})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}
