package model

import (
	"fmt"
	"strings"
)

// RuntimeEnvironment identifies the deployment environment.
type RuntimeEnvironment string

// Known deployment runtime environments.
const (
	DevelopmentRuntimeEnvironment RuntimeEnvironment = "development"
	TestRuntimeEnvironment        RuntimeEnvironment = "test"
	StagingRuntimeEnvironment     RuntimeEnvironment = "staging"
	ProductionRuntimeEnvironment  RuntimeEnvironment = "production"
)

// ParseRuntimeEnvironment validates and normalizes a runtime environment value.
func ParseRuntimeEnvironment(value string) (RuntimeEnvironment, error) {
	const op = "model.ParseRuntimeEnvironment"

	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", fmt.Errorf("%s: runtime environment is required", op)
	}

	env := RuntimeEnvironment(normalized)
	if !env.IsValid() {
		return "", fmt.Errorf("%s: unsupported runtime environment %q", op, value)
	}

	return env, nil
}

// IsValid reports whether the runtime environment is known.
func (e RuntimeEnvironment) IsValid() bool {
	switch e {
	case DevelopmentRuntimeEnvironment,
		TestRuntimeEnvironment,
		StagingRuntimeEnvironment,
		ProductionRuntimeEnvironment:
		return true

	default:
		return false
	}
}

// IsDevelopment reports whether the environment is development.
func (e RuntimeEnvironment) IsDevelopment() bool {
	return e == DevelopmentRuntimeEnvironment
}

// IsProduction reports whether the environment is production.
func (e RuntimeEnvironment) IsProduction() bool {
	return e == ProductionRuntimeEnvironment
}
