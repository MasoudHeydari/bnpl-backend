package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ApplyPaymentPendingContractExpired aligns a hosted checkout session after its
// payment_pending contract was safely expired locally. When financial safety
// still fails, the session is left unchanged so recovery can continue.
func (uc *UseCase) ApplyPaymentPendingContractExpired(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	return uc.applyPaymentPendingContractTerminal(
		ctx,
		tx,
		contractID,
		now,
		model.ExpiredCheckoutSessionStatus,
		"",
	)
}

// ApplyPaymentPendingContractCanceled aligns a hosted checkout session after its
// payment_pending contract was canceled following an authoritative Financial
// Core payment cancellation (or equivalent local safe cancel).
func (uc *UseCase) ApplyPaymentPendingContractCanceled(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	return uc.applyPaymentPendingContractTerminal(
		ctx,
		tx,
		contractID,
		now,
		model.CanceledCheckoutSessionStatus,
		model.TechnicalFailureCheckoutReason,
	)
}

func (uc *UseCase) applyPaymentPendingContractTerminal(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
	terminal model.CheckoutSessionStatus,
	reason model.CheckoutCancellationReason,
) error {
	const op = "checkoutsessionuc.UseCase.applyPaymentPendingContractTerminal"

	sessionQ := uc.checkoutSession.Tx(tx)
	locked, err := sessionQ.LockByContractID(ctx, contractID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		return fmt.Errorf("%s: lock checkout session by contract: %w", op, err)
	}

	switch locked.Status {
	case model.CanceledCheckoutSessionStatus, model.ExpiredCheckoutSessionStatus:
		return nil

	case model.CompletedCheckoutSessionStatus, model.UnavailableCheckoutSessionStatus:
		return nil

	case model.OpenCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.ProcessingCheckoutSessionStatus:
		// continue

	default:
		return nil
	}

	_, err = uc.terminateCheckoutSafelyTx(
		ctx,
		tx,
		TerminateCheckoutSafelyInput{
			SessionID:      locked.ID,
			Reason:         reason,
			TerminalStatus: terminal,
		},
		now,
	)
	if err != nil {
		// Unresolved financial effect: leave the session for recovery rather
		// than inventing a terminal transition.
		if cerr.IsCode(err, cerr.CodeConflict) {
			return nil
		}

		return err
	}

	return nil
}
