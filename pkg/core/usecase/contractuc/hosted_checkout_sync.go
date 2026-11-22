package contractuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// HostedCheckoutSync aligns a hosted CheckoutSession when a payment_pending
// contract is safely expired. Implemented by checkoutsessionuc.UseCase.
type HostedCheckoutSync interface {
	ApplyPaymentPendingContractExpired(
		ctx context.Context,
		tx repo.Tx,
		contractID model.ContractID,
		now time.Time,
	) error
}

type noopHostedCheckoutSync struct{}

func (noopHostedCheckoutSync) ApplyPaymentPendingContractExpired(
	context.Context,
	repo.Tx,
	model.ContractID,
	time.Time,
) error {
	return nil
}

// SetHostedCheckoutSync wires the hosted-checkout synchronizer after both use
// cases are constructed. A nil sync installs a noop so expiry never panics.
func (uc *UseCase) SetHostedCheckoutSync(sync HostedCheckoutSync) {
	if sync == nil {
		uc.hostedCheckoutSync = noopHostedCheckoutSync{}
		return
	}

	uc.hostedCheckoutSync = sync
}
