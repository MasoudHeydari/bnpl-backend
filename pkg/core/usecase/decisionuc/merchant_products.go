package decisionuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// merchantAllowsCreditKind checks the merchant's explicitly provisioned
// product eligibility. An inactive merchant is denied separately by the
// versioned checkout policy. This check does NOT authorize FC allocations.
//
// MerchantProfileQueryer embeds MerchantProductEligibilityQueryer, so the
// eligibility lookup is always available on the same queryer surface.
func merchantAllowsCreditKind(
	ctx context.Context, q repo.MerchantProfileQueryer,
	merchant model.MerchantProfileID, kind model.CreditKind,
) (bool, error) {
	return q.IsCreditKindEnabled(ctx, merchant, kind)
}
