package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func (uc *UseCase) resolveInitialLimitPolicy(
	ctx context.Context,
	assignmentQ repo.PolicyAssignmentQueryer,
	identitySubjectID model.IdentitySubjectID,
	now time.Time,
) (initialcreditlimit.Policy, error) {
	const op = "customercredituc.UseCase.resolveInitialLimitPolicy"

	ref := uc.defaultInitialLimitRef
	assignment, err := assignmentQ.GetActive(
		ctx,
		model.CustomerPolicySubjectType,
		identitySubjectID.String(),
		model.InitialCreditLimitPolicyFamily,
		now,
	)
	if err == nil {
		ref = policy.Ref{ID: assignment.PolicyID, Version: assignment.PolicyVersion}
	} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return nil, fmt.Errorf("%s: load active assignment: %w", op, err)
	}

	selected, err := uc.initialLimitPolicies.Resolve(ref)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve policy %s: %w", op, ref, err)
	}

	return selected, nil
}
