//go:build development && production

package buildprofile

// Intentionally undefined: development and production are mutually exclusive.
var _ = buildCannotUseDevelopmentAndProductionTagsTogether
