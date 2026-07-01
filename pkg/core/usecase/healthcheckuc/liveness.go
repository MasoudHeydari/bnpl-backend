package healthcheckuc

import (
	"context"
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// Liveness reports whether the application process is alive.
//
// Liveness intentionally avoids checking external dependencies. During
// graceful shutdown it remains healthy until the shutdown deadline is
// exceeded.
func (uc *UseCase) Liveness(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if uc.lifecycleState != nil && uc.lifecycleState.IsShutdownTimedOut() {
		return cerr.ServiceUnavailable(errors.New("application shutdown timed out"))
	}

	return nil
}
