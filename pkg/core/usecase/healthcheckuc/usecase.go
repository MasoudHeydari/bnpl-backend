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
//
// lifecycleState and dependencyChecker are required constructor dependencies.
// Passing nil panics so wiring bugs fail fast instead of soft-disabling checks.
func New(
	lifecycleState corehealth.LifecycleState,
	dependencyChecker corehealth.DependencyChecker,
) *UseCase {
	if lifecycleState == nil {
		panic("healthcheckuc: lifecycleState is nil")
	}

	if dependencyChecker == nil {
		panic("healthcheckuc: dependencyChecker is nil")
	}

	return &UseCase{
		lifecycleState:    lifecycleState,
		dependencyChecker: dependencyChecker,
	}
}
