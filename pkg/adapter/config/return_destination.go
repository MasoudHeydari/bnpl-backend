package config

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

// returnURLPolicyForEnvironment maps runtime environment onto a
// pkg/core/policy/returndestination Policy profile.
//
// Development and test allow HTTP only for localhost / loopback hosts.
// Every other environment, including unknown values, requires HTTPS.
func returnURLPolicyForEnvironment(env model.RuntimeEnvironment) returndestination.Policy {
	switch env {
	case model.DevelopmentRuntimeEnvironment, model.TestRuntimeEnvironment:
		return returndestination.Development()

	default:
		return returndestination.Strict()
	}
}
