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
// Denied/unavailable outcomes use existing unavailable semantics without OTP.
// Transient dependency failures return semantic errors; an idempotent retry
// resumes preparation on the same session when mode is on_create.
func (uc *UseCase) CreatePartner(
	ctx context.Context,
	input CreatePartnerInput,
) (CreatePartnerOutput, error) {
	if err := input.validate(); err != nil {
		return CreatePartnerOutput{}, err
	}

	session, result, err := uc.createOrReplaySession(
		ctx,
		input.CheckoutSessionRequest,
		nil,
		input.CustomerPhone,
	)
	if err != nil {
		return CreatePartnerOutput{}, err
	}

	output := CreatePartnerOutput{
		Result:      result,
		Session:     session,
		CheckoutURL: uc.hostedCheckoutURL(session.PublicToken),
	}

	if session.EffectiveOfferPreparationMode() != model.OfferPreparationOnCreate {
		return output, nil
	}

	prepared, offer, prepErr := uc.prepareExternalOffer(ctx, session)
	if prepErr != nil {
		return CreatePartnerOutput{}, prepErr
	}

	output.Session = prepared

	if offer.Outcome != "" {
		output.Offer = &offer
	}

	return output, nil
}
