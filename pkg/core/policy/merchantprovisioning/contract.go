package merchantprovisioning

import (
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
)

// ErrInvalidDeclaration indicates missing, unsupported or out-of-policy declared limits.
var ErrInvalidDeclaration = errors.New("invalid merchant self declaration")

// Policy evaluates declared financing limits and determines BNPL-owned initial terms.
type Policy interface {
	Ref() policy.Ref
	Evaluate(model.MerchantSelfDeclaration) (Terms, error)
}

// Terms are the accepted declared limits and the initial BNPL-owned account settings.
type Terms struct {
	Limits             model.MerchantSelfDeclaration
	FundingModels      []model.FundingModel
	SettlementModels   []model.SettlementModel
	Currencies         []model.Currency
	RiskTier           model.RiskTier
	RequiresBranchMode bool
	Jurisdiction       model.Jurisdiction
	CreditKinds        []model.CreditKind
}
