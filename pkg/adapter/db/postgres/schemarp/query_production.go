//go:build production

package schemarp

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
)

// DropIfExists is fail-closed in production builds. The destructive SQL
// implementation is compiled only into development binaries.
func DropIfExists[Q postgres.Queryer](_ context.Context, _ Q, _ string) error {
	return fmt.Errorf("destructive schema reset is not available in production builds")
}
