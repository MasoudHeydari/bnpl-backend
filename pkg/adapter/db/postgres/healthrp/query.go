package healthrp

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// Ping verifies that the database connection is alive.
func Ping[Q postgres.Queryer](ctx context.Context, q Q) error {
	const op = "healthrp.Ping"

	if _, err := q.Exec(ctx, "SELECT 1"); err != nil {
		return cerr.ServiceUnavailable(fmt.Errorf("%s: ping: %w", op, err))
	}

	return nil
}
