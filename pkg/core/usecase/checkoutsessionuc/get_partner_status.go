package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// GetPartnerStatus returns the authoritative merchant inquiry projection for a
// checkout the calling merchant owns.
//
// This is a read-only outcome channel. Browser return parameters are UX only
// and must never be treated as payment confirmation. Open sessions past
// expires_at are projected as expired without writing state or enqueueing
// lifecycle events; durable expiration remains the responsibility of the
// expiration worker.
func (uc *UseCase) GetPartnerStatus(
	ctx context.Context,
	input GetPartnerStatusInput,
) (GetPartnerStatusOutput, error) {
	const op = "checkoutsessionuc.UseCase.GetPartnerStatus"

	if err := input.CheckoutSessionToken.Validate(); err != nil {
		return GetPartnerStatusOutput{}, checkoutSessionTokenInvalidError(err)
	}

	if input.MerchantID.IsZero() {
		return GetPartnerStatusOutput{}, merchantIDRequiredError()
	}

	var output GetPartnerStatusOutput

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			merchantProfileQ := uc.merchantProfile.Conn(conn)
			sessionQ := uc.checkoutSession.Conn(conn)

			profile, profileErr := merchantProfileQ.GetByMerchantID(ctx, input.MerchantID)
			if profileErr != nil {
				return profileErr
			}

			found, findErr := sessionQ.GetByPublicToken(ctx, input.CheckoutSessionToken)
			if findErr != nil {
				if cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(findErr)
				}

				return findErr
			}

			// Foreign checkouts are reported as missing so a merchant cannot probe
			// for another merchant's order references.
			if found.MerchantProfileID != profile.ID {
				return checkoutSessionNotFoundError(
					fmt.Errorf("%s: checkout session is owned by another merchant", op),
				)
			}

			status := found.Status
			now := uc.clock.Now().UTC()
			if status == model.OpenCheckoutSessionStatus && found.IsExpiredAt(now) {
				status = model.ExpiredCheckoutSessionStatus
			}

			output = GetPartnerStatusOutput{
				CheckoutSessionToken: found.PublicToken,
				MerchantOrderID:      found.MerchantOrderID,
				Status:               status,
				AmountMinor:          found.RequestedAmountMinor,
				Currency:             found.Currency,
			}

			if status == model.CompletedCheckoutSessionStatus {
				output.CompletedAt = found.CompletedAt
			}

			return nil
		},
	)
	if err != nil {
		return GetPartnerStatusOutput{}, err
	}

	return output, nil
}
