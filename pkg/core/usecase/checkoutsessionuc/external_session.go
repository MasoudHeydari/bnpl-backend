package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// peekExternalSession loads an external partner checkout by public token without
// FOR UPDATE. Used to classify open vs continuation before acquiring locks in
// the canonical OTP×Saga×FC order.
func (uc *UseCase) peekExternalSession(
	ctx context.Context,
	tx repo.Tx,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.peekExternalSession"

	sessionQ := uc.checkoutSession.Tx(tx)

	found, err := sessionQ.GetByPublicToken(ctx, token)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CheckoutSession{}, checkoutSessionNotFoundError(err)
		}

		return model.CheckoutSession{}, err
	}

	if found.Origin != model.ExternalPartnerCheckoutSessionOrigin {
		return model.CheckoutSession{}, checkoutSessionNotFoundError(
			fmt.Errorf("%s: checkout session origin is %q", op, found.Origin),
		)
	}

	return found, nil
}

// lockExternalSession locks the session addressed by its public token and
// requires it to be an external partner checkout.
//
// Internal checkouts are reported as missing: the hosted external routes carry
// no authentication, so they must never act on a session that is bound to a
// trusted customer by the privileged on-behalf API.
func (uc *UseCase) lockExternalSession(
	ctx context.Context,
	tx repo.Tx,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.lockExternalSession"

	sessionQ := uc.checkoutSession.Tx(tx)

	locked, err := sessionQ.LockByPublicToken(ctx, token)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CheckoutSession{}, checkoutSessionNotFoundError(err)
		}

		return model.CheckoutSession{}, err
	}

	if locked.Origin != model.ExternalPartnerCheckoutSessionOrigin {
		return model.CheckoutSession{}, checkoutSessionNotFoundError(
			fmt.Errorf("%s: checkout session origin is %q", op, locked.Origin),
		)
	}

	return locked, nil
}

// lockExternalContinuable locks an external session that still accepts
// customer continuation. An elapsed lifetime is persisted as expired and
// reported through expired=true; the caller must commit the transaction and
// then return checkoutSessionExpiredError, because returning the error from
// the transaction would roll the expiry back.
//
// Expiry is evaluated with clock.Now() after the row lock is held so waiting
// for FOR UPDATE cannot preserve a pre-deadline eligibility snapshot.
// When expiry wins, the returned session is the persisted expired row.
func (uc *UseCase) lockExternalContinuable(
	ctx context.Context,
	tx repo.Tx,
	token model.CheckoutSessionToken,
) (locked model.CheckoutSession, expired bool, err error) {
	const op = "checkoutsessionuc.UseCase.lockExternalContinuable"

	locked, err = uc.lockExternalSession(ctx, tx, token)
	if err != nil {
		return model.CheckoutSession{}, false, err
	}

	now := uc.clock.Now().UTC()

	if !locked.Status.AllowsCustomerContinuation() {
		return model.CheckoutSession{}, false, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
		)
	}

	if locked.ContractID == nil && locked.IsExpiredAt(now) {
		persisted, transitionErr := uc.transitionToExpired(ctx, tx, locked, now)
		if transitionErr != nil {
			return model.CheckoutSession{}, false, transitionErr
		}

		return persisted, true, nil
	}

	return locked, false, nil
}

// lockExternalOpenForOTPIssuance locks an open external session for initial OTP
// SMS issuance. Confirming/processing continuation must lock Saga→Contract→PA
// before calling lockExternalSession — see hostedCheckoutOTPAuthorizer.
func (uc *UseCase) lockExternalOpenForOTPIssuance(
	ctx context.Context,
	tx repo.Tx,
	token model.CheckoutSessionToken,
) (locked model.CheckoutSession, expired bool, err error) {
	const op = "checkoutsessionuc.UseCase.lockExternalOpenForOTPIssuance"

	locked, err = uc.lockExternalSession(ctx, tx, token)
	if err != nil {
		return model.CheckoutSession{}, false, err
	}

	if locked.Status != model.OpenCheckoutSessionStatus {
		return model.CheckoutSession{}, false, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
		)
	}

	now := uc.clock.Now().UTC()

	if locked.ContractID == nil && locked.IsExpiredAt(now) {
		persisted, transitionErr := uc.transitionToExpired(ctx, tx, locked, now)
		if transitionErr != nil {
			return model.CheckoutSession{}, false, transitionErr
		}

		return persisted, true, nil
	}

	return locked, false, nil
}

// allowsExternalConfirmWithOTP reports whether a public Confirm that just
// consumed a fresh OTP may enter Confirm() for the given session status.
// Completed checkouts are observed via confirmation-status, not re-confirmed.
func allowsExternalConfirmWithOTP(status model.CheckoutSessionStatus) bool {
	switch status {
	case model.OpenCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.ProcessingCheckoutSessionStatus:
		return true

	default:
		return false
	}
}
