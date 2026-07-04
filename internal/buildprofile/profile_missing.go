//go:build !development && !production

package buildprofile

// Intentionally undefined: every build must explicitly select one profile.
var _ = buildRequiresExactlyOneOfDevelopmentOrProductionTag
