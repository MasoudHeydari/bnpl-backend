package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MerchantProductEligibilityQueryer returns the merchant's specifically
// authorized credit products. An absent monthly eligibility is a DENIAL,
// not implicit permission to reuse installment credit.
type MerchantProductEligibilityQueryer interface {
	IsCreditKindEnabled(context.Context,model.MerchantProfileID,model.CreditKind)(bool,error)
}

// MerchantProductEligibilityTxQueryer is an authorization-scoped configuration
// command to be called by the merchant administration use case. It does not
// affect existing contracts or Financial Core's accounting permissions.
type MerchantProductEligibilityTxQueryer interface {
	MerchantProductEligibilityQueryer
	SetCreditKindEnabled(context.Context,model.MerchantProfileID,model.CreditKind,bool,time.Time) error
}
