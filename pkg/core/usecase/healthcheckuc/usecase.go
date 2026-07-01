package healthcheckuc

import (
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
)

// UseCase evaluates the application's readiness and liveness state.
type UseCase struct {
	lifecycleState    corehealth.LifecycleState
	dependencyChecker corehealth.DependencyChecker
}

// New constructs a health check use case.
func New(
	lifecycleState corehealth.LifecycleState,
	dependencyChecker corehealth.DependencyChecker,
) *UseCase {
	return &UseCase{
		lifecycleState:    lifecycleState,
		dependencyChecker: dependencyChecker,
	}
}
