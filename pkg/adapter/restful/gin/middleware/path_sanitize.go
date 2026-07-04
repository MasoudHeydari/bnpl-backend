package middleware

import (
	"net/url"
	"regexp"
	"strings"
)

// checkoutPublicTokenPathPattern matches an opaque hosted checkout public token
// segment (cs_ + 64 lowercase hex) embedded in an HTTP path.
var checkoutPublicTokenPathPattern = regexp.MustCompile(`/cs_[0-9a-f]{64}(/|$)`)

// SanitizeHTTPPathForLog redacts checkout public tokens from diagnostic path
// values so development request logs and 5xx error logs do not retain full
// cs_<64 hex> locators. Route templates from c.FullPath() need no redaction.
func SanitizeHTTPPathForLog(path string) string {
	if path == "" {
		return path
	}

	return checkoutPublicTokenPathPattern.ReplaceAllStringFunc(path, func(match string) string {
		if len(match) > 0 && match[len(match)-1] == '/' {
			return "/cs_[REDACTED]/"
		}

		return "/cs_[REDACTED]"
	})
}

// SanitizeHTTPRefererForLog redacts checkout public tokens from a Referer URL
// while preserving a usable origin/path shape for diagnostics. Unparseable
// values fall back to path-only sanitization so a raw cs_ token cannot leak.
func SanitizeHTTPRefererForLog(referer string) string {
	if referer == "" {
		return referer
	}

	parsed, err := url.Parse(referer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return SanitizeHTTPPathForLog(referer)
	}

	// Rebuild without url.URL.String() so the redaction marker is not percent-encoded.
	var b strings.Builder
	b.WriteString(parsed.Scheme)
	b.WriteString("://")
	b.WriteString(parsed.Host)
	b.WriteString(SanitizeHTTPPathForLog(parsed.Path))

	return b.String()
}
