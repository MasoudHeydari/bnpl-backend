package config

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Runtime contains root deployment runtime settings.
type Runtime struct {
	// Environment is the deployment runtime environment name.
	Environment string `yaml:"environment"`
}

// ParsedEnvironment returns the validated runtime environment.
func (r Runtime) ParsedEnvironment() (model.RuntimeEnvironment, error) {
	return model.ParseRuntimeEnvironment(r.Environment)
}
