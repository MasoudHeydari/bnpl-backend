package customercredituc

import "github.com/parstis/bnpl-backend/pkg/core/model"

// AssignInitialLimitOutput returns the credit limit decision and optional limit assignment.
type AssignInitialLimitOutput struct {
	// Result reports the use-case outcome classification.
	Result AssignInitialLimitResult

	// Decision is the persisted decision record.
	Decision model.Decision
	// CreditLimit is the assigned customer credit limit when created.
	CreditLimit *model.CreditAccount
}
