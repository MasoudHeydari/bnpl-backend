package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Start evaluates financing for the customer BNPL just authenticated on the
// hosted checkout page (internal / first-party Hosted flow).
//
// The authenticated customer must be the customer already attached to the
// session (trusted internal on-behalf binding). A mismatch stops the checkout
// instead of silently financing a different person. External Partner Hosted
// checkouts prepare the offer during Partner Create and use StartExternal only
// to request OTP — they do not call this method.
func (uc *UseCase) Start(ctx context.Context, input StartInput) (StartOutput, error) {
	if err := input.validate(); err != nil {
		return StartOutput{}, err
	}

	session, err := uc.bindCustomerToSession(ctx, input.CheckoutSessionID, input.ExternalCustomerID)
	if err != nil {
		return StartOutput{}, err
	}

	prepared, offer, err := uc.prepareOfferForBoundSession(ctx, session, input.ExternalCustomerID)
	if err != nil {
		return StartOutput{}, err
	}

	return StartOutput{Session: prepared, Offer: offer}, nil
}

// prepareOfferForBoundSession creates or replays the financing Decision/Offer for
// a session whose customer identity is already resolved/bound.
//
// It is shared by internal authenticated Start and External Partner Create
// offer preparation. It does not send OTP and does not authorize a purchase.
func (uc *UseCase) prepareOfferForBoundSession(
	ctx context.Context,
	session model.CheckoutSession,
	externalCustomerID model.ExternalCustomerID,
) (model.CheckoutSession, Offer, error) {
	if session.ContractID != nil {
		// Payment retry must present the original accepted terms. The Decision
		// and allocation cannot be regenerated after Contract creation.
		existing, getErr := uc.Get(ctx, GetInput{
			CheckoutSessionID:  session.ID,
			ExternalCustomerID: externalCustomerID,
		})
		if getErr != nil {
			return model.CheckoutSession{}, Offer{}, getErr
		}

		if existing.Offer == nil {
			return model.CheckoutSession{}, Offer{}, checkoutOfferNotReadyError(
				fmt.Errorf("checkoutsessionuc.prepareOfferForBoundSession: accepted terms are missing"),
			)
		}

		return existing.Session, *existing.Offer, nil
	}

	evaluated, offer, err := uc.evaluateSession(ctx, session, externalCustomerID)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	return evaluated, offer, nil
}

// bindCustomerToSession verifies the authenticated customer owns the session
// and attaches the customer to it.
//
// An elapsed lifetime is persisted as expired before the caller is rejected, so
// the recorded state matches what the merchant sees when polling.
func (uc *UseCase) bindCustomerToSession(
	ctx context.Context,
	id model.CheckoutSessionID,
	externalCustomerID model.ExternalCustomerID,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.bindCustomerToSession"

	var (
		session model.CheckoutSession
		expired bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)
					customerProfileQ := uc.customerProfile.Tx(tx)

					locked, lockErr := sessionQ.LockByID(ctx, id)
					if lockErr != nil {
						if cerr.IsCode(lockErr, cerr.CodeResourceNotFound) {
							return checkoutSessionNotFoundError(lockErr)
						}

						return lockErr
					}

					if !locked.Status.AllowsCustomerContinuation() {
						return checkoutSessionNotContinuableError(
							fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
						)
					}

					now := uc.clock.Now().UTC()
					if locked.ContractID == nil && locked.IsExpiredAt(now) {
						if _, statusErr := uc.transitionToExpired(ctx, tx, locked, now); statusErr != nil {
							return statusErr
						}

						expired = true

						return nil
					}

					profile, profileErr := customerProfileQ.LockByExternalCustomerID(ctx, externalCustomerID)
					if profileErr != nil {
						return profileErr
					}

					if bindingErr := assertBoundCustomer(locked, profile.ID); bindingErr != nil {
						return bindingErr
					}

					bound, bindErr := sessionQ.BindCustomer(ctx, locked.ID, profile.ID, now)
					if bindErr != nil {
						return bindErr
					}

					session = bound

					return nil
				},
			)
		},
	)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if expired {
		return model.CheckoutSession{}, checkoutSessionExpiredError()
	}

	return session, nil
}

// assertBoundCustomer reports whether customerID is the customer this checkout
// belongs to.
//
// Internal checkouts are pre-bound by the privileged on-behalf API. External
// partner checkouts carry a customer after Identity resolves the
// Merchant-asserted verified phone (during Partner Create preparation). That
// binding is Identity resolution, not OTP possession proof or purchase
// authorization. A session with no resolved customer fails closed: the
// merchant's customer_reference is never an identity mapping.
func assertBoundCustomer(session model.CheckoutSession, customerID model.CustomerID) error {
	const op = "checkoutsessionuc.assertBoundCustomer"

	if session.CustomerID == nil {
		return checkoutCustomerIdentityConflictError(
			fmt.Errorf("%s: %s session has no resolved customer", op, session.Origin),
		)
	}

	if *session.CustomerID != customerID {
		return checkoutCustomerIdentityConflictError(fmt.Errorf("%s: session belongs to another customer", op))
	}

	return nil
}
