package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ensureOfferPreparedForStart loads the checkout and prepares the financing
// offer when it is not already linked.
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

			session = found

			return nil
		},
	)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	return uc.ensureOfferPreparedForStartSession(ctx, session)
}

// ensureOfferPreparedForStartSession prepares the financing offer when it is
// not already linked. Preparation timing is primarily decided at Partner Create
// (on_create vs on_start); START still safely prepares a missing offer so
// deferred sessions and interrupted on_create retries converge on the same
// state machine without duplicating Decisions.
//
// Origin and continuability are validated before any preparation side effects.
func (uc *UseCase) ensureOfferPreparedForStartSession(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.ensureOfferPreparedForStartSession"

	if session.Origin != model.ExternalPartnerCheckoutSessionOrigin {
		return model.CheckoutSession{}, checkoutSessionNotFoundError(
			fmt.Errorf("%s: checkout session origin is %q", op, session.Origin),
		)
	}

	if session.Status != model.OpenCheckoutSessionStatus {
		return model.CheckoutSession{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout session status is %q", op, session.Status),
		)
	}

	if session.ContractID != nil && !session.ContractID.IsZero() {
		return model.CheckoutSession{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout already has a contract", op),
		)
	}

	now := uc.clock.Now().UTC()
	if session.IsExpiredAt(now) {
		return model.CheckoutSession{}, checkoutSessionExpiredError()
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

	// Hosted START cannot choose or override the credit kind; Partner
	// SelectCreditKind owns that selection before the customer reaches OTP.
	selected, err := uc.requireSelectedCreditKindForStart(ctx, input.Token)
	if err != nil {
		return StartExternalOutput{}, err
	}

	prepared, err := uc.ensureOfferPreparedForStartSession(ctx, selected)
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

	reconfirmRequired := false
	if prepared.EvaluationGeneration != input.EvaluationGeneration {
		// Stale generation: soft reconfirm with refreshed offer.
		reconfirmRequired = true
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

	// Product-scoped denials keep the session open with a denied offer so the
	// customer can select another eligible product. Session-terminal denials
	// already moved status to unavailable above.
	if offer.Outcome != ApprovedOfferOutcome && !reconfirmRequired {
		if prepared.Status == model.OpenCheckoutSessionStatus {
			return StartExternalOutput{
				Session:              prepared,
				EvaluationGeneration: prepared.EvaluationGeneration,
				Offer:                offer,
			}, nil
		}

		return StartExternalOutput{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: offer outcome is %q", op, offer.Outcome),
		)
	}

	now := uc.clock.Now().UTC()
	if offer.OfferExpiresAt != nil && !offer.OfferExpiresAt.After(now) && offer.Outcome == ApprovedOfferOutcome {
		return StartExternalOutput{}, checkoutSessionExpiredError()
	}

	if prepared.IsExpiredAt(now) {
		return StartExternalOutput{}, checkoutSessionExpiredError()
	}

	return StartExternalOutput{
		Session:              prepared,
		EvaluationGeneration: prepared.EvaluationGeneration,
		Offer:                offer,
		ReconfirmRequired:    reconfirmRequired,
	}, nil
}

// bindExternalCustomer attaches an already-registered BNPL customer (resolved
// via CRS phone → external_customer_id) to the session.
//
// This runs during Partner Create / deferred START offer preparation. Binding
// records Customer Reference resolution, not OTP possession proof or purchase
// authorization. Profiles are never created here: a CRS customer without a
// BNPL profile is a deterministic financing rejection. A session already bound
// to a different customer fails closed.
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

					profile, err := customerProfileQ.GetByExternalCustomerID(ctx, externalCustomerID)
					if err != nil {
						if cerr.IsCode(err, cerr.CodeResourceNotFound) {
							return checkoutCreditPurchaseUnavailableForPhoneError(
								fmt.Errorf("%s: bnpl customer profile not found", op),
							)
						}

						return err
					}

					if locked.CustomerID != nil && *locked.CustomerID != profile.ID {
						return checkoutCustomerIdentityConflictError(
							fmt.Errorf("%s: session belongs to another customer", op),
						)
					}

					now := uc.clock.Now().UTC()

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
