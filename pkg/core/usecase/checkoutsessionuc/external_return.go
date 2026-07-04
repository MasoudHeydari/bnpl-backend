package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ReturnExternal builds the browser return URL for an external hosted checkout.
//
// The destination comes exclusively from the ReturnURL resolved and persisted
// at CheckoutSession creation from a pre-registered merchant return destination.
// Request query parameters, Referer, and Origin are never consulted. This
// endpoint is browser UX only: it is not payment authority and must not be
// treated as merchant order fulfillment truth.
func (uc *UseCase) ReturnExternal(
	ctx context.Context,
	input ReturnExternalInput,
) (ReturnExternalOutput, error) {
	const op = "checkoutsessionuc.UseCase.ReturnExternal"

	if err := input.validate(); err != nil {
		return ReturnExternalOutput{}, err
	}

	var destination string

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			sessionQ := uc.checkoutSession.Conn(conn)

			found, err := sessionQ.GetByPublicToken(ctx, input.Token)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(err)
				}

				return err
			}

			if found.Origin != model.ExternalPartnerCheckoutSessionOrigin {
				return checkoutSessionNotFoundError(
					fmt.Errorf("%s: checkout session origin is %q", op, found.Origin),
				)
			}

			url, err := merchantReturnURL(found)
			if err != nil {
				return err
			}

			destination = url

			return nil
		},
	)
	if err != nil {
		return ReturnExternalOutput{}, err
	}

	return ReturnExternalOutput{RedirectURL: destination}, nil
}
