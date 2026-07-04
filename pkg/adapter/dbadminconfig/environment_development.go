//go:build development

package dbadminconfig

// Development tools may inspect any valid deployment environment. Destructive
// initialization still has the existing separate Development-only environment gate.
func validateBuildEnvironment(string) error { return nil }
