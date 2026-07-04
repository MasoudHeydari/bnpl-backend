package merchantuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// GetProfile returns the authenticated merchant's BNPL profile and display attributes.
//
// Missing merchant profiles are resource-not-found; this endpoint does not
// auto-provision merchants. Client-safe error messages/details are produced in
// the application layer without referring to HTTP headers or transport types.
func (uc *UseCase) GetProfile(ctx context.Context, input GetProfileInput) (GetProfileOutput, error) {
	if input.MerchantID.IsZero() {
		return GetProfileOutput{}, merchantIDRequiredError()
	}

	var output GetProfileOutput
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantProfileQ := uc.merchantProfile.Conn(conn)
		profile, err := merchantProfileQ.GetByMerchantID(ctx, input.MerchantID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return merchantProfileNotFoundError(err)
			}

			return err
		}

		output = GetProfileOutput{
			MerchantID:         profile.MerchantID,
			DisplayName:        profile.DisplayName,
			LegalName:          profile.LegalName,
			LogoURL:            profile.LogoURL,
			SupportPhone:       profile.SupportPhone,
			SupportEmail:       profile.SupportEmail,
			Status:             profile.Status,
			FundingModels:      profile.FundingModels,
			SettlementModels:   profile.SettlementModels,
			Currencies:         profile.Currencies,
			MaxAmountMinor:     profile.MaxAmountMinor,
			MaxAmountCurrency:  profile.MaxAmountCurrency,
			MaxTenureMonths:    profile.MaxTenureMonths,
			RequiresBranchMode: profile.RequiresBranchMode,
			Jurisdiction:       profile.Jurisdiction,
			CreatedAt:          profile.CreatedAt,
			UpdatedAt:          profile.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return GetProfileOutput{}, err
	}

	return output, nil
}
