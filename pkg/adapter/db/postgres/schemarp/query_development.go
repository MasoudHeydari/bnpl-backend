//go:build development

package schemarp

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
)

// DropIfExists drops schema and all contained objects when it exists.
// This operation is destructive and is used only by init-dev.
func DropIfExists[Q postgres.Queryer](
	ctx context.Context, q Q, schema string,
) error {
	// Schema name is a trusted application constant (bnpl).
	_, err := q.Exec(
		ctx,
		fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, schema),
	)
	return err
}
