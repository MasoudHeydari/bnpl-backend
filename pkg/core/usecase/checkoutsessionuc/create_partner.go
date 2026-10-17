package checkoutsessionuc

import (
	"context"
)

// CreatePartner creates the hosted handoff an external merchant redirects to,
// then prepares the customer-specific financing offer before returning.
//
// Preparation resolves the Merchant-asserted verified phone through Identity,
// binds the resolved customer, and creates/replays the Decision/Offer. It does
// not send OTP, create a Contract, create a PaymentAttempt, or call Financial
// Core for money movement.
//
// The Partner response stays minimal: checkout id, status, hosted URL, and
// expiry. Offer economics, customer identifiers, and credit facts never cross
// this boundary — they belong to the customer's Hosted BNPL experience.
//
// A successful open status means the approved offer is durably prepared so
// Hosted GET can render it read-only. Denied/unavailable outcomes use existing
// unavailable semantics without OTP. Transient dependency failures return
// semantic errors; an idempotent retry resumes preparation on the same session.
//
// Financial Core TenantID/ContextID are resolved from the authenticated
// merchant's server-owned MerchantProfile payment scope and snapshotted onto
// the CheckoutSession so Hosted Confirm can reconstruct payment context without
// browser JWT headers or Partner Gateway identity headers.
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

	prepared, err := uc.prepareExternalOffer(ctx, session)
	if err != nil {
		return CreatePartnerOutput{}, err
	}

	return CreatePartnerOutput{
		Result:      result,
		Session:     prepared,
		CheckoutURL: uc.hostedCheckoutURL(prepared.PublicToken),
	}, nil
}
