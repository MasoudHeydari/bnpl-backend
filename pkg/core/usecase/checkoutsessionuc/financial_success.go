package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutverificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutverification/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ApplyPaymentSucceeded records durable Financial Core success on the linked
// checkout as verify_pending. It never marks the merchant as having verified.
//
// Late success against a terminal non-financially-successful session is a
// reconciliation conflict. Already financially successful rows are idempotent.
func (uc *UseCase) ApplyPaymentSucceeded(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	const op = "checkoutsessionuc.UseCase.ApplyPaymentSucceeded"

	return uc.markCheckoutVerifyPending(ctx, tx, contractID, now, op)
}

func (uc *UseCase) markCheckoutVerifyPending(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
	op string,
) error {
	sessionQ := uc.checkoutSession.Tx(tx)

	locked, err := sessionQ.LockByContractID(ctx, contractID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		return fmt.Errorf("%s: lock checkout session by contract: %w", op, err)
	}

	switch locked.Status {
	case model.VerifyPendingCheckoutSessionStatus,
		model.VerifiedByMerchantCheckoutSessionStatus,
		model.CancellationPendingCheckoutSessionStatus:
		return nil

	case model.ProcessingCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.OpenCheckoutSessionStatus:
		// Normal and recoverable paths into verify_pending.

	default:
		return uc.recordCheckoutPaymentStateConflict(
			ctx,
			tx,
			locked,
			contractID,
			now,
		)
	}

	if transitionErr := locked.Status.ValidateTransitionTo(model.VerifyPendingCheckoutSessionStatus); transitionErr != nil {
		return uc.recordCheckoutPaymentStateConflict(
			ctx,
			tx,
			locked,
			contractID,
			now,
		)
	}

	policy := checkoutverificationv1.New()
	pendingAt := now.UTC()
	deadlineAt := policy.DeadlineAt(pendingAt)

	updated, err := sessionQ.MarkVerifyPending(
		ctx,
		locked.ID,
		pendingAt,
		deadlineAt,
		policy.Version(),
		pendingAt,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeConflict) {
			replay, getErr := sessionQ.GetByID(ctx, locked.ID)
			if getErr != nil {
				return fmt.Errorf("%s: reload after verify_pending conflict: %w", op, getErr)
			}

			if replay.Status.IsFinanciallySuccessful() {
				return nil
			}
		}

		return err
	}

	return uc.lifecycle.Enqueue(
		ctx,
		tx,
		updated,
		checkoutlifecycle.EventTypeVerifyPending,
		now,
	)
}
