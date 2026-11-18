package merchantuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// creditKindAdminKinds is the stable response order of administered products.
var creditKindAdminKinds = []model.CreditKind{
	model.PayInInstallmentsCreditKind,
	model.PayNextMonthCreditKind,
}

// ListCreditKinds returns the effective eligibility of every credit kind for a
// merchant. Both kinds are disabled until explicitly enabled.
//
// The transport layer must have required permission.MerchantsReadAny. The
// result always contains one entry per kind, so an unconfigured merchant is a
// successful response rather than not-found.
func (uc *CreditKindUseCase) ListCreditKinds(
	ctx context.Context,
	input ListCreditKindsInput,
) (ListCreditKindsOutput, error) {
	const op = "merchantuc.CreditKindUseCase.ListCreditKinds"

	if input.MerchantProfileID.IsZero() {
		return ListCreditKindsOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: merchant profile id is required", op),
		)
	}

	output := ListCreditKindsOutput{
		MerchantProfileID: input.MerchantProfileID,
		CreditKinds:       make([]CreditKindEligibility, 0, len(creditKindAdminKinds)),
	}

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantProfileQ := uc.merchantProfile.Conn(conn)

		profile, err := merchantProfileQ.GetByID(ctx, input.MerchantProfileID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return merchantProfileNotFoundError(err)
			}

			return fmt.Errorf("%s: get merchant profile: %w", op, err)
		}

		for _, kind := range creditKindAdminKinds {
			enabled, err := merchantProfileQ.IsCreditKindEnabled(ctx, profile.ID, kind)
			if err != nil {
				return fmt.Errorf("%s: read eligibility for %s: %w", op, kind, err)
			}

			output.CreditKinds = append(output.CreditKinds, CreditKindEligibility{
				CreditKind: kind,
				Enabled:    enabled,
			})
		}

		return nil
	})
	if err != nil {
		return ListCreditKindsOutput{}, err
	}

	return output, nil
}
