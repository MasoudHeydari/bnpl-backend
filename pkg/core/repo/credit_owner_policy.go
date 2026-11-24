package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreditOwnerPolicyAssignmentQueryer resolves the immutable legal person's
// assigned policy through the authenticated CRS account id. It is deliberately
// read-only: owner-level grants are written via the normal Tx Create port.
type CreditOwnerPolicyAssignmentQueryer interface {
	GetActiveForCreditOwner(ctx context.Context, externalCustomerID model.ExternalCustomerID, family model.PolicyFamily, now time.Time) (model.PolicyAssignment, error)
}

// GetActiveCustomerOrOwnerPolicy preserves legacy explicit customer assignments
// when the legal person has no governing assignment. Once a person is admitted
// to the trusted cohort, their policy is shared across every enrolled account.
func GetActiveCustomerOrOwnerPolicy(
	ctx context.Context,
	q PolicyAssignmentQueryer,
	customer model.ExternalCustomerID,
	family model.PolicyFamily,
	now time.Time,
) (model.PolicyAssignment, error) {
	if ownerQ, ok := q.(CreditOwnerPolicyAssignmentQueryer); ok {
		row, err := ownerQ.GetActiveForCreditOwner(ctx, customer, family, now)
		if err == nil {
			return row, nil
		}

		if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PolicyAssignment{}, err
		}
	}

	return q.GetActive(
		ctx,
		model.CustomerPolicySubjectType,
		customer.String(),
		family,
		now,
	)
}
