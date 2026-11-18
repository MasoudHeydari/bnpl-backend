package merchantuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// creditProductKinds is the stable response order of administered products.
var creditProductKinds = []model.CreditKind{
	model.PayIn4CreditKind,
	model.PayNextMonthCreditKind,
}

// ListCreditProducts returns the effective eligibility of every credit product
// for a merchant, applying the same defaults checkout uses (pay_in_4 enabled
// when absent, pay_next_month disabled unless explicitly enabled).
//
// The transport layer must have required permission.MerchantsReadAny. The
// result always contains one entry per product, so an unconfigured merchant is
// a successful response rather than not-found.
func (uc *CreditProductUseCase) ListCreditProducts(
	ctx context.Context,
	input ListCreditProductsInput,
) (ListCreditProductsOutput, error) {
	const op = "merchantuc.CreditProductUseCase.ListCreditProducts"

	if input.MerchantProfileID.IsZero() {
		return ListCreditProductsOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: merchant profile id is required", op),
		)
	}

	output := ListCreditProductsOutput{
		MerchantProfileID: input.MerchantProfileID,
		Products:          make([]CreditProductEligibility, 0, len(creditProductKinds)),
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

		for _, kind := range creditProductKinds {
			enabled, err := merchantProfileQ.IsCreditKindEnabled(ctx, profile.ID, kind)
			if err != nil {
				return fmt.Errorf("%s: read eligibility for %s: %w", op, kind, err)
			}

			output.Products = append(output.Products, CreditProductEligibility{
				CreditKind: kind,
				Enabled:    enabled,
			})
		}

		return nil
	})
	if err != nil {
		return ListCreditProductsOutput{}, err
	}

	return output, nil
}
