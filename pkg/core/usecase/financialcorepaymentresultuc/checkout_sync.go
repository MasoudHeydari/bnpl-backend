package financialcorepaymentresultuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// SyncCheckoutAfterRetryablePaymentTerminal returns a processing CheckoutSession
// to open after an authoritative FAILED/CANCELED PaymentAttempt. The
// payment_pending Contract stays so the customer can Confirm again. Must run in
// the same local transaction as the attempt transition.
func (uc *UseCase) SyncCheckoutAfterRetryablePaymentTerminal(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	return uc.checkoutSessionUC.ApplyPaymentRetryableFailure(ctx, tx, contractID, now)
}

// SyncCheckoutAfterPaymentPendingCanceled aligns the hosted CheckoutSession after
// a payment_pending contract was safely terminated locally (not after a mere
// PaymentAttempt cancellation). Prefer SyncCheckoutAfterRetryablePaymentTerminal
// when only the attempt ended and the purchase may continue.
func (uc *UseCase) SyncCheckoutAfterPaymentPendingCanceled(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	return uc.checkoutSessionUC.ApplyPaymentPendingContractCanceled(ctx, tx, contractID, now)
}
