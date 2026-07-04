package healthcheckuc

import (
	"context"
)

// Readiness reports whether the application can safely accept new work.
//
// Readiness fails while the application is draining or when a required
// dependency, such as the database, cache, etc. is unavailable.
func (uc *UseCase) Readiness(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if uc.lifecycleState.IsDraining() {
		return applicationDrainingError()
	}

	if err := uc.dependencyChecker.Check(ctx); err != nil {
		return err
	}

	return nil
}
