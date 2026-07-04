package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestGetExternalConfirmationStatus_NotStartedBeforeConfirm(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	session := h.session(withoutDecision())
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, model.OpenCheckoutSessionStatus, got.Status)
	require.Equal(t, ConfirmationStatusNotStarted, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionStartConfirm, got.NextAction)
	require.Nil(t, got.RetryAfterSeconds)
}

func TestGetExternalConfirmationStatus_Completed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	session := h.session(withStatus(model.CompletedCheckoutSessionStatus), withContract(h.contractID))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusCompleted, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionShowSuccess, got.NextAction)
}

func TestGetExternalConfirmationStatus_ConfirmingIsProcessing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// Session lookup + optional saga lookup when ConfirmationAttemptID is set.
	h.expectPool(2, 0)

	session := h.session(withStatus(model.ConfirmingCheckoutSessionStatus))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusProcessing, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionWait, got.NextAction)
	require.NotNil(t, got.RetryAfterSeconds)
}

func TestGetExternalConfirmationStatus_RejectsMalformedToken(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: "bad"})
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.checkout_session_id_invalid")
}

func TestGetExternalConfirmationStatus_InternalOriginNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(h.session(withOrigin(model.InternalCheckoutSessionOrigin)), nil).Once()

	_, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func TestGetExternalConfirmationStatus_ProjectsElapsedOpenAsExpired(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	session := h.session(withoutDecision(), withExpiresAt(h.now.Add(-time.Second)))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, model.ExpiredCheckoutSessionStatus, got.Status)
	require.Equal(t, ConfirmationStatusExpired, got.ConfirmationStatus)
}
