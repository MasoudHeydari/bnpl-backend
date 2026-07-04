package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreateInternal creates a checkout session for a trusted first-party caller and
// immediately evaluates financing for the supplied customer identity.
//
// The customer identity is trusted here because the route requires the on-behalf
// capability in addition to checkout creation. External partners cannot reach
// this path and therefore cannot name a customer.
func (uc *UseCase) CreateInternal(
	ctx context.Context,
	input CreateInternalInput,
) (CreateInternalOutput, error) {
	const op = "checkoutsessionuc.UseCase.CreateInternal"

	if err := input.validate(); err != nil {
		return CreateInternalOutput{}, err
	}

	profile, err := uc.customerProfileByIdentitySubjectID(ctx, input.IdentitySubjectID)
	if err != nil {
		return CreateInternalOutput{}, err
	}

	customerID := profile.ID

	session, result, err := uc.createOrReplaySession(ctx, input.CheckoutSessionRequest, &customerID, "", nil, nil)
	if err != nil {
		return CreateInternalOutput{}, err
	}

	// A replayed merchant order reference must belong to the same customer.
	// Reusing it for someone else is a conflict, not a new evaluation.
	if session.CustomerID == nil || *session.CustomerID != customerID {
		return CreateInternalOutput{}, merchantOrderConflictError(
			fmt.Errorf("%s: merchant order id reused for a different customer", op),
		)
	}

	// Terminal or non-continuable sessions must not be re-evaluated. Replaying
	// evaluation against completed/canceled/processing would either 500 on an
	// illegal transition or mutate decision_id after the checkout finished.
	if !session.Status.AllowsCustomerContinuation() {
		return CreateInternalOutput{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout session status is %q", op, session.Status),
		)
	}

	evaluated, offer, err := uc.prepareOfferForBoundSession(ctx, session, input.IdentitySubjectID)
	if err != nil {
		return CreateInternalOutput{}, err
	}

	return CreateInternalOutput{
		Result:      result,
		Session:     evaluated,
		CheckoutURL: uc.hostedCheckoutURL(evaluated.PublicToken),
		Offer:       offer,
	}, nil
}

// customerProfileByIdentitySubjectID loads the BNPL customer profile for an
// external Identity Service subject. Checkout foreign keys use the resolved
// internal CustomerID (customer_profiles.id), not the Identity subject.
func (uc *UseCase) customerProfileByIdentitySubjectID(
	ctx context.Context,
	identitySubjectID model.IdentitySubjectID,
) (model.CustomerProfile, error) {
	var profile model.CustomerProfile

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			customerProfileQ := uc.customerProfile.Conn(conn)

			found, findErr := customerProfileQ.GetByIdentitySubjectID(ctx, identitySubjectID)
			if findErr != nil {
				return findErr
			}

			profile = found

			return nil
		},
	)
	if err != nil {
		return model.CustomerProfile{}, err
	}

	return profile, nil
}
