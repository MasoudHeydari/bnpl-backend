package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// transitionToExpired marks an open session expired and enqueues the lifecycle
// event only when the status actually changes. Lazy expiry and the background
// job share this helper so merchants observe one logical expired event.
func (uc *UseCase) transitionToExpired(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
	now time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.transitionToExpired"

	sessionQ := uc.checkoutSession.Tx(tx)

	if session.Status == model.ExpiredCheckoutSessionStatus {
		return session, nil
	}

	if session.Status.BlocksCancelAndExpiry() {
		return model.CheckoutSession{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: checkout session status is %q", op, session.Status),
		)
	}

	if session.Status != model.OpenCheckoutSessionStatus {
		return model.CheckoutSession{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout session status is %q", op, session.Status),
		)
	}

	if transitionErr := session.Status.ValidateTransitionTo(model.ExpiredCheckoutSessionStatus); transitionErr != nil {
		return model.CheckoutSession{}, cerr.InternalError(fmt.Errorf("%s: %w", op, transitionErr))
	}

	expired, err := sessionQ.UpdateStatus(ctx, session.ID, model.ExpiredCheckoutSessionStatus, now)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if enqueueErr := uc.lifecycle.Enqueue(
		ctx, tx, expired, checkoutlifecycle.EventTypeExpired, now,
	); enqueueErr != nil {
		return model.CheckoutSession{}, enqueueErr
	}

	return expired, nil
}
