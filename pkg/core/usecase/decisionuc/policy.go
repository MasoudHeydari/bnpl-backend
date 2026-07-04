package decisionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func (uc *UseCase) resolveCheckoutPolicy(
	ctx context.Context,
	assignmentQ repo.PolicyAssignmentQueryer,
	externalCustomerID model.ExternalCustomerID,
	now time.Time,
) (checkout.Policy, error) {
	const op = "decisionuc.UseCase.resolveCheckoutPolicy"

	ref := uc.defaultCheckoutPolicyRef
	assignment, err := assignmentQ.GetActive(
		ctx,
		model.CustomerPolicySubjectType,
		externalCustomerID.String(),
		model.CheckoutPolicyFamily,
		now,
	)
	if err == nil {
		ref = policy.Ref{ID: assignment.PolicyID, Version: assignment.PolicyVersion}
	} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return nil, fmt.Errorf("%s: load active assignment: %w", op, err)
	}

	selected, err := uc.checkoutPolicies.Resolve(ref)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve policy %s: %w", op, ref, err)
	}

	return selected, nil
}

func (uc *UseCase) resolveCheckoutPolicyRef(ref policy.Ref) (checkout.Policy, error) {
	const op = "decisionuc.UseCase.resolveCheckoutPolicyRef"

	selected, err := uc.checkoutPolicies.Resolve(ref)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve persisted policy %s: %w", op, ref, err)
	}

	return selected, nil
}
