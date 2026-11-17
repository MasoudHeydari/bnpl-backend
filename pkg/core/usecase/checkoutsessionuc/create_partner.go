package checkoutsessionuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreatePartner creates a CheckoutSession for any merchant authenticated by
// terminal credentials, then optionally prepares the financing offer.
//
// Offer preparation timing follows the trusted
// merchant_branch_profiles.offer_preparation_mode snapshotted onto the session
// at create time:
//   - on_create: resolve the merchant-asserted phone through the Customer
//     Reference Service, bind the customer, prepare/replay the Decision/Offer,
//     and return the Offer to the merchant together with the checkout URL.
//   - on_start: persist the handoff only; offer preparation and OTP happen on
//     the unified customer START path.
//
// Create never sends OTP, creates a Contract, creates a PaymentAttempt, or
// calls Financial Core for money movement. OTP remains mandatory on Confirm for
// every merchant type.
//
// Phone financing rejections (CRS miss/suspend or missing BNPL profile) and
// missing credit assessment return public Forbidden errors after the session is
// terminalized. Other denied Decisions keep unavailable + denied Offer
// semantics. Transient dependency failures return semantic errors; an
// idempotent retry resumes preparation on the same session when mode is
// on_create.
func (uc *UseCase) CreatePartner(
	ctx context.Context,
	input CreatePartnerInput,
) (CreatePartnerOutput, error) {
	if err := input.validate(); err != nil {
		uc.observer.SessionInputRejected(ctx, "invalid_input")
		return CreatePartnerOutput{}, err
	}

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

	output := CreatePartnerOutput{
		Result:      result,
		Session:     session,
		CheckoutURL: uc.hostedCheckoutURL(session.PublicToken),
	}

	// Unspecified product means the hosted customer chooses at START, even
	// when merchant configuration previously requested on_create preparation.
	// Explicit legacy product checkouts retain their prior on_create behavior.
	if input.CreditKind == "" || session.EffectiveOfferPreparationMode() != model.OfferPreparationOnCreate {
		return output, nil
	}

	uc.observer.OfferPreparationStarted(ctx, session.ID)

	prepared, offer, prepErr := uc.prepareExternalOffer(ctx, session)
	if prepErr != nil {
		uc.observer.OfferPreparationFailed(ctx, session.ID, "offer_preparation_failed")
		return CreatePartnerOutput{}, prepErr
	}

	output.Session = prepared

	if offer.Outcome != "" {
		output.Offer = &offer

		var decisionID model.DecisionID
		if prepared.DecisionID != nil {
			decisionID = *prepared.DecisionID
		}

		uc.observer.OfferPrepared(ctx, prepared.ID, decisionID)
	}

	return output, nil
}
