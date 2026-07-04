//go:build production

package migrationsuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// initProdRequiresProductionError returns a public validation error when
// init-prod is invoked outside production.
//
// The concrete environment value is retained only in cause for diagnostics.
func initProdRequiresProductionError(env model.RuntimeEnvironment) error {
	const op = "migrationsuc.initProdRequiresProductionError"

	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.init_prod_requires_runtime_environment_production",
		fmt.Errorf(
			"%s: init-prod requires runtime.environment=production (APP_ENVIRONMENT=production), got %q", op, env,
		),
		cerr.Details{
			Fields: []string{"environment"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.init_prod_requires_runtime_environment_production",
					Fields:     []string{"environment"},
				},
			},
		},
	)
}

// schemaAlreadyExistsConflictError returns a public conflict when production
// init refuses to recreate an existing managed schema.
//
// Fields stay empty because this is a resource-state conflict, not a field
// validation failure. cause is preserved for diagnostics only.
func schemaAlreadyExistsConflictError(schemaName string) error {
	const op = "migrationsuc.schemaAlreadyExistsConflictError"

	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.request_conflicts_with_the_current_resource_state",
		fmt.Errorf(
			"%s: managed schema %q already exists; refusing to recreate — use `bnpl db migrate` (make migrate) for schema changes", op, schemaName,
		),
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.request_conflicts_with_the_current_resource_state",
					Fields:     []string{},
				},
			},
		},
	)
}
