package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreatePartner creates a CheckoutSession for any merchant authenticated by
// terminal credentials, resolves the merchant-asserted phone through CRS,
// binds the BNPL customer when usable, and returns a read-only dual credit-kind
// eligibility preview. The Merchant website selects a kind afterward via
// SelectCreditKind.
//
// Create never sends OTP, creates a Decision/Contract/PaymentAttempt, or calls
// Financial Core for money movement. Offer preparation runs after Partner
// SelectCreditKind (and Hosted START when still deferred).
//
// Phone financing rejections (CRS miss/suspend or missing BNPL profile)
// terminalize the session and return a public Forbidden error. Credit-kind
// unavailability is reported in CreditKinds, not as a create failure.
func (uc *UseCase) CreatePartner(
	ctx context.Context,
	input CreatePartnerInput,
) (CreatePartnerOutput, error) {
	if err := input.validate(); err != nil {
		uc.observer.SessionInputRejected(ctx, "invalid_input")
		return CreatePartnerOutput{}, err
	}

	// Partner Create must not accept a credit-kind selection. Selection is a
	// separate authenticated Partner command after the merchant UI presents
	// the eligibility preview.
	input.CreditKind = ""

	uc.observer.SessionCreateRequested(ctx, input.ExternalMerchantID)

	session, result, err := uc.createOrReplaySession(
		ctx,
		input.CheckoutSessionRequest,
		nil,
		input.CustomerPhone,
	)
	if err != nil {
		return CreatePartnerOutput{}, err
	}

	switch result {
	case CreateResultAlreadyExists:
		uc.observer.SessionReplayed(ctx, session.ID)

	default:
		uc.observer.SessionCreated(ctx, session.ID)
	}

	uc.observer.HostedHandoffPrepared(ctx, session.ID)

	bound, bindErr := uc.ensureExternalCustomerBoundForPreview(ctx, session)
	if bindErr != nil {
		return CreatePartnerOutput{}, bindErr
	}

	previews, previewErr := uc.buildCreditKindsPreview(ctx, bound)
	if previewErr != nil {
		return CreatePartnerOutput{}, previewErr
	}

	return CreatePartnerOutput{
		Result:      result,
		Session:     bound,
		CheckoutURL: uc.hostedCheckoutURL(bound.PublicToken),
		CreditKinds: previews,
	}, nil
}

// ensureExternalCustomerBoundForPreview resolves CRS and binds the customer
// without creating a Decision. Used by Partner Create so eligibility preview
// can inspect credit accounts. SelectCreditKind prepares the offer later.
func (uc *UseCase) ensureExternalCustomerBoundForPreview(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.ensureExternalCustomerBoundForPreview"

	if session.Origin != model.ExternalPartnerCheckoutSessionOrigin {
		return model.CheckoutSession{}, cerr.InternalError(
			fmt.Errorf("%s: expected external partner origin, got %q", op, session.Origin),
		)
	}

	if !session.Status.AllowsCustomerContinuation() {
		if session.Status == model.UnavailableCheckoutSessionStatus && session.CustomerID == nil {
			return session, checkoutCreditPurchaseUnavailableForPhoneError(
				fmt.Errorf("%s: replay of phone financing rejection", op),
			)
		}

		return session, nil
	}

	if session.CustomerID != nil && !session.CustomerID.IsZero() {
		// Partner Create can replay a bound Checkout after the merchant's
		// phone is reassigned. Do not return a person-specific eligibility
		// preview without verifying the current CRS binding.
		if err := uc.ensureBoundSessionMobileOwnership(ctx, session); err != nil {
			if isOpaqueMobileOwnershipFailure(err) {
				return model.CheckoutSession{}, checkoutCreditPurchaseUnavailableForPhoneError(err)
			}

			return model.CheckoutSession{}, err
		}

		return session, nil
	}

	now := uc.clock.Now().UTC()
	if session.IsExpiredAt(now) {
		expired, err := uc.persistExternalExpired(ctx, session.PublicToken)
		if err != nil {
			return model.CheckoutSession{}, err
		}

		return expired, checkoutSessionExpiredError()
	}

	phone, err := uc.decryptCustomerPhone(session)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	externalCustomerID, usable, err := uc.resolveExternalCustomerID(ctx, session, phone)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if !usable {
		unavailable, unavailableErr := uc.persistExternalUnavailable(ctx, session.PublicToken)
		if unavailableErr != nil {
			return model.CheckoutSession{}, unavailableErr
		}

		return unavailable, checkoutCreditPurchaseUnavailableForPhoneError(
			fmt.Errorf("%s: customer reference did not yield a usable customer", op),
		)
	}

	bound, err := uc.bindExternalCustomer(ctx, session.PublicToken, externalCustomerID)
	if err != nil {
		if isCreditPurchaseUnavailableForPhone(err) {
			unavailable, unavailableErr := uc.persistExternalUnavailable(ctx, session.PublicToken)
			if unavailableErr != nil {
				return model.CheckoutSession{}, unavailableErr
			}

			return unavailable, err
		}

		return model.CheckoutSession{}, err
	}

	return bound, nil
}
