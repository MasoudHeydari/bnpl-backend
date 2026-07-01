package healthcheckuc

import (
	"context"
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// Readiness reports whether the application can safely accept new work.
//
// Readiness fails while the application is draining or when a required
// dependency, such as the database, cache, etc. is unavailable.
func (uc *UseCase) Readiness(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if uc.lifecycleState != nil && uc.lifecycleState.IsDraining() {
		return cerr.ServiceUnavailable(errors.New("application is draining"))
	}

	if err := uc.dependencyChecker.Check(ctx); err != nil {
		return err
	}

	return nil
}
