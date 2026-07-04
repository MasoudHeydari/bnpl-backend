package financialcorepaymentresultuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// checkoutSessionPaymentHook advances CheckoutSession state from authoritative
// Financial Core payment outcomes. Implemented by checkoutsessionuc.UseCase.
type checkoutSessionPaymentHook interface {
	ApplyPaymentSucceeded(
		ctx context.Context,
		tx repo.Tx,
		contractID model.ContractID,
		now time.Time,
	) error

	ApplyPaymentRetryableFailure(
		ctx context.Context,
		tx repo.Tx,
		contractID model.ContractID,
		now time.Time,
	) error
}

// noopCheckoutSessionPaymentHook ignores session transitions. Unit tests that
// do not assert checkout-session side effects inject this collaborator.
type noopCheckoutSessionPaymentHook struct{}

func (noopCheckoutSessionPaymentHook) ApplyPaymentSucceeded(
	context.Context,
	repo.Tx,
	model.ContractID,
	time.Time,
) error {
	return nil
}

func (noopCheckoutSessionPaymentHook) ApplyPaymentRetryableFailure(
	context.Context,
	repo.Tx,
	model.ContractID,
	time.Time,
) error {
	return nil
}
