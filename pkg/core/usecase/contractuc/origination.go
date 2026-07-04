package contractuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// requireOriginationEnabledJurisdiction fails closed when
// MerchantProfile.Jurisdiction may not originate new BNPL business in this
// deployment. Call only on the new first-Contract path after proving no
// Contract exists for the Decision. Never use this for replay, resume, or
// post-Contract servicing.
func (uc *UseCase) requireOriginationEnabledJurisdiction(
	op string,
	jurisdiction model.Jurisdiction,
) error {
	if err := uc.jurisdictions.RequireOriginationEnabled(jurisdiction); err != nil {
		return cerr.InternalError(fmt.Errorf(
			"%s: merchant jurisdiction origination is not enabled for this deployment: %w",
			op,
			err,
		))
	}

	return nil
}
