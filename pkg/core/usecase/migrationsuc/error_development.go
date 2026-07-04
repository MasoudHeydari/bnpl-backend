//go:build development

package migrationsuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// initDevRequiresDevelopmentError returns a public validation error when
// init-dev is invoked outside development.
//
// The concrete environment value is retained only in cause for diagnostics.
func initDevRequiresDevelopmentError(env model.RuntimeEnvironment) error {
	const op = "migrationsuc.initDevRequiresDevelopmentError"

	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.init_dev_requires_runtime_environment_development",
		fmt.Errorf(
			"%s: init-dev requires runtime.environment=development (APP_ENVIRONMENT=development), got %q", op, env,
		),
		cerr.Details{
			Fields: []string{"environment"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.init_dev_requires_runtime_environment_development",
					Fields:     []string{"environment"},
				},
			},
		},
	)
}
