package checkoutsessionuc

import (
	"context"
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UpsertReturnDestination registers a merchant browser-return destination.
//
// Checkout create selects destinations by symbolic name only. Arbitrary return
// URLs are never accepted on the checkout request.
func (uc *UseCase) UpsertReturnDestination(
	ctx context.Context,
	input UpsertReturnDestinationInput,
) (UpsertReturnDestinationOutput, error) {
	if err := input.validate(uc.returnURLPolicy); err != nil {
		return UpsertReturnDestinationOutput{}, err
	}

	var destination model.MerchantReturnDestination

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					merchantProfileQ := uc.merchantProfile.Tx(tx)
					sessionQ := uc.checkoutSession.Tx(tx)

					merchant, merchantErr := merchantProfileQ.GetByMerchantID(ctx, input.MerchantID)
					if merchantErr != nil {
						return merchantErr
					}

					now := uc.clock.Now().UTC()

					saved, saveErr := sessionQ.UpsertReturnDestination(
						ctx,
						merchant.MerchantID,
						input.DestinationName,
						input.ReturnURL,
						input.Enabled,
						now,
					)
					if saveErr != nil {
						return saveErr
					}

					destination = saved

					return nil
				},
			)
		},
	)
	if err != nil {
		return UpsertReturnDestinationOutput{}, err
	}

	return UpsertReturnDestinationOutput{Destination: destination}, nil
}

func (i UpsertReturnDestinationInput) validate(policy returndestination.Policy) error {
	if i.MerchantID.IsZero() {
		return merchantIDRequiredError()
	}

	// Destination names are canonical identifiers: no trim/lowercase rewriting.
	if err := policy.ValidateName(i.DestinationName); err != nil {
		return mapDestinationNameError(err, "destination_name")
	}

	if err := policy.ValidateURL(i.ReturnURL); err != nil {
		return mapReturnURLError(err)
	}

	return nil
}

func mapDestinationNameError(err error, field string) error {
	switch {
	case errors.Is(err, returndestination.ErrDestinationNameEmpty):
		if field == "destination_name" {
			return destinationNameRequiredError()
		}

		return returnDestinationRequiredError()

	case errors.Is(err, returndestination.ErrDestinationNameTooLong):
		if field == "destination_name" {
			return destinationNameTooLongError()
		}

		return returnDestinationTooLongError()

	default:
		if field == "destination_name" {
			return destinationNameInvalidError()
		}

		return returnDestinationInvalidError()
	}
}

func mapReturnURLError(err error) error {
	switch {
	case errors.Is(err, returndestination.ErrReturnURLEmpty):
		return returnURLRequiredError()

	case errors.Is(err, returndestination.ErrReturnURLTooLong):
		return returnURLTooLongError()

	case errors.Is(err, returndestination.ErrInsecureScheme):
		return returnURLMustUseHTTPSError()

	default:
		return returnURLInvalidError()
	}
}
