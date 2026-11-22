package financialcorepaymentresultuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// SyncCheckoutAfterPaymentPendingCanceled aligns the hosted CheckoutSession after
// a payment_pending contract was canceled from an authoritative Financial Core
// payment cancellation. It must run in the same local transaction.
func (uc *UseCase) SyncCheckoutAfterPaymentPendingCanceled(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	return uc.checkoutSessionUC.ApplyPaymentPendingContractCanceled(ctx, tx, contractID, now)
}
