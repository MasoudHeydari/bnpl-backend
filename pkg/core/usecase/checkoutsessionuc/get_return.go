package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// GetReturn returns the browser-return projection for the customer bound to a
// checkout session.
//
// The projection carries the session state for hosted-page polling plus the
// pre-registered merchant return URL. The URL contains only checkout_session_id
// and the opaque merchant state; it never encodes payment truth. A session owned
// by a different customer is reported as missing.
func (uc *UseCase) GetReturn(ctx context.Context, input GetReturnInput) (GetReturnOutput, error) {
	const op = "checkoutsessionuc.UseCase.GetReturn"

	if input.CheckoutSessionID.IsZero() {
		return GetReturnOutput{}, checkoutSessionIDRequiredError()
	}

	if input.ExternalCustomerID.IsZero() {
		return GetReturnOutput{}, externalCustomerIDRequiredError()
	}

	var session model.CheckoutSession

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			sessionQ := uc.checkoutSession.Conn(conn)
			customerProfileQ := uc.customerProfile.Conn(conn)

			found, findErr := sessionQ.GetByID(ctx, input.CheckoutSessionID)
			if findErr != nil {
				if cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(findErr)
				}

				return findErr
			}

			profile, profileErr := customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
			if profileErr != nil {
				return profileErr
			}

			// Only a session already bound to this customer is readable. Unlike
			// cancel-before-start, return never invents a customer identity.
			if found.CustomerID == nil || *found.CustomerID != profile.ID {
				return checkoutSessionNotFoundError(
					fmt.Errorf("%s: checkout session is not bound to this customer", op),
				)
			}

			session = found

			return nil
		},
	)
	if err != nil {
		return GetReturnOutput{}, err
	}

	returnURL, err := merchantReturnURL(session)
	if err != nil {
		return GetReturnOutput{}, err
	}

	return GetReturnOutput{Session: session, ReturnURL: returnURL}, nil
}
