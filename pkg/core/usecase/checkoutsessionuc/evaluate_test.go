package checkoutsessionuc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestBumpEvaluationGenerationCAS(t *testing.T) {
	t.Parallel()

	t.Run("winning bump returns the bumped session", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.expectPool(0, 1)

		current := h.session(withGeneration(1))
		bumped := h.session(withGeneration(2), withoutDecision())

		h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(current, nil).Once()
		h.sessionTx.EXPECT().
			BumpEvaluationGenerationCAS(
				h.ctx,
				h.sessionID,
				1,
				current.DecisionID,
				h.now,
			).
			Return(bumped, nil).Once()

		got, err := h.uc.bumpEvaluationGenerationCAS(h.ctx, current)
		require.NoError(t, err)
		require.Equal(t, 2, got.EvaluationGeneration)
	})

	lostRace := []struct {
		name string
		err  error
	}{
		{name: "conflict", err: cerr.Conflict(errors.New("generation moved"))},
		{name: "row no longer matches the CAS", err: cerr.ResourceNotFound(errors.New("no matching open row"))},
	}

	for _, tt := range lostRace {
		t.Run("lost race reloads winner on "+tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(0, 1)

			stale := h.session(withGeneration(1))
			newDecisionID := model.DecisionID(uuid.New())
			winner := h.session(withGeneration(2), func(s *model.CheckoutSession) { s.DecisionID = &newDecisionID })

			h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(stale, nil).Once()
			h.sessionTx.EXPECT().
				BumpEvaluationGenerationCAS(
					h.ctx,
					h.sessionID,
					1,
					stale.DecisionID,
					h.now,
				).
				Return(model.CheckoutSession{}, tt.err).Once()

			// The loser reuses the winner's row instead of bumping again.
			h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(winner, nil).Once()

			got, err := h.uc.bumpEvaluationGenerationCAS(h.ctx, stale)
			require.NoError(t, err)
			require.Equal(t, 2, got.EvaluationGeneration)
			require.Equal(t, &newDecisionID, got.DecisionID)
		})
	}

	t.Run("other bump failures are propagated without reload", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.expectPool(0, 1)

		stale := h.session(withGeneration(1))
		cause := errors.New("db down")

		h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(stale, nil).Once()
		h.sessionTx.EXPECT().
			BumpEvaluationGenerationCAS(
				h.ctx,
				h.sessionID,
				1,
				stale.DecisionID,
				h.now,
			).
			Return(model.CheckoutSession{}, cause).Once()

		_, err := h.uc.bumpEvaluationGenerationCAS(h.ctx, stale)
		require.ErrorIs(t, err, cause)
	})
}

func TestStart_UnavailableSessionIsNotReEvaluatedAndEmitsNoEvent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	unavailable := h.session(withStatus(model.UnavailableCheckoutSessionStatus))

	// A duplicate start after a denied evaluation stops at the continuation
	// guard: no decision, no UpdateStatus, no second unavailable event.
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(unavailable, nil).Once()

	_, err := h.uc.Start(h.ctx, StartInput{
		CheckoutSessionID:  h.sessionID,
		ExternalCustomerID: h.profileID,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
	require.Empty(t, h.decisions.calls)
}

func TestRecordEvaluation_UnavailableTransitionEmitsOneEvent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	open := h.session()
	linked := h.session()
	unavailable := h.session(withStatus(model.UnavailableCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().LinkDecision(
		h.ctx,
		h.sessionID,
		h.decisionID,
		1,
		h.now,
	).Return(linked, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.UnavailableCheckoutSessionStatus, h.now).
		Return(unavailable, nil).Once()
	h.expectLifecycleEvent(unavailable, checkoutlifecycle.EventTypeUnavailable)

	got, err := h.uc.recordEvaluation(
		h.ctx,
		h.sessionID,
		1,
		h.decisionID,
		model.UnavailableCheckoutSessionStatus,
		checkoutlifecycle.EventTypeUnavailable,
	)
	require.NoError(t, err)
	require.Equal(t, model.UnavailableCheckoutSessionStatus, got.Status)
}

func TestRecordEvaluation_ExpiredBeforeDecisionLinkPersistsExpiry(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	open := h.session(withExpiresAt(h.now))
	expired := h.session(withExpiresAt(h.now), withStatus(model.ExpiredCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.recordEvaluation(
		h.ctx,
		h.sessionID,
		1,
		h.decisionID,
		model.OpenCheckoutSessionStatus,
		"",
	)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
}

func TestRecordEvaluation_ConcurrentUnavailableWinnerDoesNotEmitSecondEvent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	open := h.session()

	// A concurrent evaluation already moved the row to unavailable between the
	// guard and LinkDecision: the status is not rewritten and no event is
	// enqueued by this caller (UpdateStatus / outbox are unexpected).
	alreadyUnavailable := h.session(withStatus(model.UnavailableCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		LinkDecision(
			h.ctx,
			h.sessionID,
			h.decisionID,
			1,
			h.now,
		).
		Return(alreadyUnavailable, nil).Once()

	got, err := h.uc.recordEvaluation(
		h.ctx,
		h.sessionID,
		1,
		h.decisionID,
		model.UnavailableCheckoutSessionStatus,
		checkoutlifecycle.EventTypeUnavailable,
	)
	require.NoError(t, err)
	require.Equal(t, model.UnavailableCheckoutSessionStatus, got.Status)
}

func TestRecordEvaluation_AlreadyUnavailableReplaysWithoutWrite(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	unavailable := h.session(withStatus(model.UnavailableCheckoutSessionStatus))
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(unavailable, nil).Once()

	got, err := h.uc.recordEvaluation(
		h.ctx,
		h.sessionID,
		1,
		h.decisionID,
		model.UnavailableCheckoutSessionStatus,
		checkoutlifecycle.EventTypeUnavailable,
	)
	require.NoError(t, err)
	require.Equal(t, unavailable, got)
}

func TestRecordEvaluation_RefusesToLinkDecisionOntoNonOpenSession(t *testing.T) {
	t.Parallel()

	statuses := []model.CheckoutSessionStatus{
		model.ConfirmingCheckoutSessionStatus,
		model.ProcessingCheckoutSessionStatus,
		model.CompletedCheckoutSessionStatus,
		model.CanceledCheckoutSessionStatus,
		model.ExpiredCheckoutSessionStatus,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(0, 1)

			h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(h.session(withStatus(status)), nil).Once()

			_, err := h.uc.recordEvaluation(
				h.ctx,
				h.sessionID,
				1,
				h.decisionID,
				model.OpenCheckoutSessionStatus,
				"",
			)
			requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
		})
	}
}

func TestLifecycleOutbox_EventIDIsStableForOneLogicalTransition(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	unavailable := h.session(withStatus(model.UnavailableCheckoutSessionStatus))

	var messages []repo.CreateOutboxMessage

	h.merchantTx.EXPECT().
		GetByID(h.ctx, unavailable.MerchantProfileID).
		Return(model.MerchantProfile{ID: unavailable.MerchantProfileID, MerchantID: h.merchantID}, nil).Twice()
	h.sessionTx.EXPECT().
		GetWebhookDestination(h.ctx, h.merchantID).
		Return(model.MerchantWebhookDestination{}, cerr.ResourceNotFound(errors.New("no webhook"))).Twice()
	h.outboxTx.EXPECT().
		Create(h.ctx, testifymock.Anything).
		RunAndReturn(func(_ context.Context, message repo.CreateOutboxMessage) (model.OutboxMessage, error) {
			messages = append(messages, message)

			return model.OutboxMessage{}, nil
		}).Twice()

	// Retrying the same logical transition reuses one event id so merchants and
	// the outbox deduplicate instead of seeing independent unavailable events.
	require.NoError(t, h.uc.lifecycle.Enqueue(
		h.ctx,
		h.tx,
		unavailable,
		checkoutlifecycle.EventTypeUnavailable,
		h.now,
	))
	require.NoError(t, h.uc.lifecycle.Enqueue(
		h.ctx,
		h.tx,
		unavailable,
		checkoutlifecycle.EventTypeUnavailable,
		h.now,
	))

	require.Len(t, messages, 2)
	require.Equal(t, messages[0].Payload, messages[1].Payload)
	require.Contains(
		t,
		string(messages[0].Payload),
		`"event_id":"`+unavailable.PublicToken.String()+`:`+checkoutlifecycle.EventTypeUnavailable+`:unavailable"`,
	)
}

// Contract acceptance freezes the Decision generation, even after expiry.
// This guards the Start/Prepare retry path from silently changing accepted
// financing terms after a previous payment attempt has failed.
func TestPrepareEvaluationGeneration_AcceptedContractNeverBumps(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	accepted := h.session(withContract(h.contractID))
	out, err := h.uc.prepareEvaluationGeneration(h.ctx, accepted)
	require.NoError(t, err)
	require.Equal(t, accepted, out)
	// No repository calls are expected on this path.
}
