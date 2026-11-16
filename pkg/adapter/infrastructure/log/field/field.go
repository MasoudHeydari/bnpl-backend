// Package field provides allowlisted structured log field constructors and
// sanitization for the slog Logger adapter.
//
// Business log attributes must go through these constructors (or Core
// log.TextAttr for operational keys). The Logger adapter still validates every
// Field via Sanitize before emission.
package field
