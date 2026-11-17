package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/repo"

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

	// Preserve on_create behavior for merchants supporting only pay-in-four.
	// When monthly is also enabled, the customer must choose at Hosted START
	// before the immutable Decision is created. Replays retain prior offers.
	if session.EffectiveOfferPreparationMode() != model.OfferPreparationOnCreate {
		return output, nil
	}
	if input.CreditKind == "" && session.DecisionID == nil {
		monthlyEnabled, eligibilityErr := uc.merchantAllowsMonthlyCredit(ctx, session)
		if eligibilityErr != nil {
			return CreatePartnerOutput{}, eligibilityErr
		}
		if monthlyEnabled {
			return output, nil
		}
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

// merchantAllowsMonthlyCredit reads the merchant's product provisioning.
// Legacy test adapters without this optional read capability fall back to
// pay-in-four; they must never implicitly authorize monthly financing.
func (uc *UseCase) merchantAllowsMonthlyCredit(
	ctx context.Context, session model.CheckoutSession,
) (bool, error) {
	var enabled bool
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		productQ, ok := uc.merchantProfile.Conn(conn).(repo.MerchantProductEligibilityQueryer)
		if !ok {
			enabled = false
			return nil
		}
		var lookupErr error
		enabled, lookupErr = productQ.IsCreditKindEnabled(
			ctx, session.MerchantProfileID, model.PayNextMonthCreditKind,
		)
		if lookupErr != nil {
			return cerr.InternalError(fmt.Errorf("merchant product eligibility: %w", lookupErr))
		}
		return nil
	})
	return enabled, err
}
