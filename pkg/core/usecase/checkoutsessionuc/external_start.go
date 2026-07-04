package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ensureOfferPreparedForStart loads the checkout and prepares the financing
// offer when it is not already linked. Preparation timing is primarily decided
// at Partner Create (on_create vs on_start); START still safely prepares a
// missing offer so deferred sessions and interrupted on_create retries converge
// on the same state machine without duplicating Decisions.
//
// Origin and continuability are validated before any preparation side effects.
func (uc *UseCase) ensureOfferPreparedForStart(
	ctx context.Context,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.ensureOfferPreparedForStart"

	var session model.CheckoutSession

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			sessionQ := uc.checkoutSession.Conn(conn)

			found, findErr := sessionQ.GetByPublicToken(ctx, token)
			if findErr != nil {
				if cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(findErr)
				}

				return findErr
			}

			if found.Origin != model.ExternalPartnerCheckoutSessionOrigin {
				return checkoutSessionNotFoundError(
					fmt.Errorf("%s: checkout session origin is %q", op, found.Origin),
				)
			}

			if found.Status != model.OpenCheckoutSessionStatus {
				return checkoutSessionNotContinuableError(
					fmt.Errorf("%s: checkout session status is %q", op, found.Status),
				)
			}

			if found.ContractID != nil && !found.ContractID.IsZero() {
				return checkoutSessionNotContinuableError(
					fmt.Errorf("%s: checkout already has a contract", op),
				)
			}

			session = found

			return nil
		},
	)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if session.CustomerID != nil && session.DecisionID != nil && !session.DecisionID.IsZero() {
		return session, nil
	}

	prepared, _, err := uc.prepareExternalOffer(ctx, session)
	return prepared, err
}

// StartExternal ensures the financing offer is prepared for Hosted Checkout.
//
// When the offer was deferred (on_start), the first START prepares it. Clients
// that have not yet seen a generation may omit evaluation_generation (zero);
// START returns the Offer and authoritative evaluation_generation. When the
// client supplies a generation, it must match the durable session.
//
// START never generates, issues, verifies, or sends OTP and never calls
// Notification Service or Financial Core.
func (uc *UseCase) StartExternal(
	ctx context.Context,
	input StartExternalInput,
) (StartExternalOutput, error) {
	const op = "checkoutsessionuc.UseCase.StartExternal"

	if err := input.validate(); err != nil {
		return StartExternalOutput{}, err
	}

	prepared, err := uc.ensureOfferPreparedForStart(ctx, input.Token)
	if err != nil {
		return StartExternalOutput{}, err
	}

	// Deferred first START: client has not yet observed a generation.
	if input.EvaluationGeneration < 1 {
		input.EvaluationGeneration = prepared.EvaluationGeneration
	}

	if input.EvaluationGeneration < 1 {
		return StartExternalOutput{}, checkoutOfferNotReadyError(
			fmt.Errorf("%s: offer preparation did not produce an evaluation generation", op),
		)
	}

	if prepared.EvaluationGeneration != input.EvaluationGeneration {
		return StartExternalOutput{}, checkoutOfferReconfirmRequiredError(
			fmt.Errorf(
				"%s: displayed generation %d does not match current %d",
				op,
				input.EvaluationGeneration,
				prepared.EvaluationGeneration,
			),
		)
	}

	if prepared.Status != model.OpenCheckoutSessionStatus {
		return StartExternalOutput{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout session status is %q", op, prepared.Status),
		)
	}

	if prepared.ContractID != nil && !prepared.ContractID.IsZero() {
		return StartExternalOutput{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout already has a contract", op),
		)
	}

	if prepared.CustomerID == nil {
		return StartExternalOutput{}, checkoutOfferNotReadyError(
			fmt.Errorf("%s: customer identity is not resolved", op),
		)
	}

	if prepared.DecisionID == nil || prepared.DecisionID.IsZero() {
		return StartExternalOutput{}, checkoutOfferNotReadyError(
			fmt.Errorf("%s: financing decision is not prepared", op),
		)
	}

	offer, err := uc.projectPersistedOffer(ctx, prepared)
	if err != nil {
		return StartExternalOutput{}, err
	}

	if offer.Outcome != ApprovedOfferOutcome {
		return StartExternalOutput{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: offer outcome is %q", op, offer.Outcome),
		)
	}

	now := uc.clock.Now().UTC()
	if offer.OfferExpiresAt != nil && !offer.OfferExpiresAt.After(now) {
		return StartExternalOutput{}, checkoutSessionExpiredError()
	}

	if prepared.IsExpiredAt(now) {
		return StartExternalOutput{}, checkoutSessionExpiredError()
	}

	return StartExternalOutput{
		Session:              prepared,
		EvaluationGeneration: prepared.EvaluationGeneration,
		Offer:                offer,
	}, nil
}

// bindExternalCustomer attaches the customer resolved by the Identity Service
// to the session, creating the BNPL profile when this is its first checkout.
//
// This runs during Partner Create offer preparation. Binding records Identity
// resolution (phone → customer), not OTP possession proof or purchase
// authorization. A session already bound to a different customer fails closed.
func (uc *UseCase) bindExternalCustomer(
	ctx context.Context,
	token model.CheckoutSessionToken,
	externalCustomerID model.ExternalCustomerID,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.bindExternalCustomer"

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

					locked, lockedExpired, err := uc.lockExternalContinuable(ctx, tx, token)
					if err != nil {
						return err
					}

					if lockedExpired {
						expired = true

						return nil
					}

					now := uc.clock.Now().UTC()

					profile, _, err := customerProfileQ.EnsureExists(
						ctx,
						externalCustomerID,
						model.IRJurisdiction,
						model.ActiveCustomerStatus,
						now,
						now,
					)
					if err != nil {
						return err
					}

					if locked.CustomerID != nil && *locked.CustomerID != profile.ID {
						return checkoutCustomerIdentityConflictError(
							fmt.Errorf("%s: session belongs to another customer", op),
						)
					}

					bound, err := sessionQ.BindCustomer(ctx, locked.ID, profile.ID, now)
					if err != nil {
						return err
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

// merchantDisplayName loads the trusted BNPL merchant display name for OTP
// notification templating. Partner request labels are never used.
func (uc *UseCase) merchantDisplayName(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
) (string, error) {
	const op = "checkoutsessionuc.UseCase.merchantDisplayName"

	var name string

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantQ := uc.merchantProfile.Conn(conn)
		merchant, getErr := merchantQ.GetByID(ctx, merchantProfileID)
		if getErr != nil {
			return getErr
		}

		name = merchant.DisplayName

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("%s: load merchant profile: %w", op, err)
	}

	if name == "" {
		return "", cerr.InternalError(fmt.Errorf("%s: merchant display name is empty", op))
	}

	return name, nil
}
