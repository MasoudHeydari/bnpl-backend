package decisionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// merchantAllowsCreditKind checks the merchant's explicitly provisioned
// product eligibility. An inactive merchant is denied separately by the
// versioned checkout policy. This check does NOT authorize FC allocations.
func merchantAllowsCreditKind(
	ctx context.Context,q repo.MerchantProfileQueryer,
	merchant model.MerchantProfileID,kind model.CreditKind,
)(bool,error){
	eligibility,ok:=q.(repo.MerchantProductEligibilityQueryer)
	if !ok{
		return false,cerr.InternalError(fmt.Errorf("merchant product eligibility repository unavailable"))
	}
	return eligibility.IsCreditKindEnabled(ctx,merchant,kind)
}
