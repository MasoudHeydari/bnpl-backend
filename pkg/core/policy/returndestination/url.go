package returndestination

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

const (
	// MaxReturnURLLength bounds a registered merchant return URL.
	MaxReturnURLLength = 2048
)

var (
	// ErrReturnURLEmpty means the return URL is missing.
	ErrReturnURLEmpty = errors.New("return url is required")

	// ErrReturnURLTooLong means the return URL exceeds MaxReturnURLLength.
	ErrReturnURLTooLong = errors.New("return url is too long")

	// ErrReturnURLInvalid means the return URL violates structural rules.
	ErrReturnURLInvalid = errors.New("return url is invalid")

	// ErrInsecureScheme means HTTPS is required but the URL uses another scheme,
	// or HTTP was used with a non-loopback host under Development.
	ErrInsecureScheme = errors.New("return url must use https")
)

// Policy controls scheme requirements for registered return URLs.
//
// Structural rules (absolute URL, hostname, no userinfo/query/fragment) always
// apply. Policy selection is performed by Config/Bootstrap from the runtime
// environment; this type stays pure and deterministic.
//
// Construct via Strict / Development rather than assembling fields by hand.
type Policy struct {
	allowHTTPForLoopback bool
}

// Strict returns the production/staging return-URL policy: HTTPS only.
func Strict() Policy {
	return Policy{allowHTTPForLoopback: false}
}

// Development returns the development/test return-URL policy: HTTPS is always
// allowed, and HTTP is allowed only for localhost / loopback hosts so local
// receivers such as http://localhost:3000/bnpl/return remain usable.
func Development() Policy {
	return Policy{allowHTTPForLoopback: true}
}

// ValidateName reports whether name is a canonical symbolic return destination
// identifier. Name grammar is independent of scheme policy.
func (Policy) ValidateName(name string) error {
	return ValidateDestinationName(name)
}

// ValidateURL reports whether raw is an acceptable merchant return URL under p.
//
// Rules:
//   - required, max length MaxReturnURLLength
//   - absolute http(s) URL with hostname
//   - no userinfo, fragment, query string (including empty "?" / "#"), or
//     control characters
//   - leading/trailing whitespace is rejected (not silently trimmed)
//   - https always accepted when structurally valid
//   - http accepted only when Development and host is loopback
func (p Policy) ValidateURL(raw string) error {
	if raw == "" {
		return ErrReturnURLEmpty
	}

	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\t") {
		return ErrReturnURLInvalid
	}

	if len(raw) > MaxReturnURLLength {
		return ErrReturnURLTooLong
	}

	// Reject query/fragment delimiters before parsing so empty "?" / "#" cannot
	// be normalized away (ForceQuery / empty Fragment).
	if strings.ContainsAny(raw, "?#") {
		return ErrReturnURLInvalid
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Opaque != "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawQuery != "" {
		return ErrReturnURLInvalid
	}

	switch parsed.Scheme {
	case "https":
		return nil

	case "http":
		if !p.allowHTTPForLoopback {
			return ErrInsecureScheme
		}

		if !isLoopbackHost(parsed.Hostname()) {
			return ErrInsecureScheme
		}

		return nil

	default:
		return ErrReturnURLInvalid
	}
}

// isLoopbackHost reports whether host is exactly "localhost" or a loopback IP
// literal. DNS names that merely contain "localhost" (for example
// localhost.example.com) are rejected. No DNS resolution is performed.
// Trailing-dot hostnames are not treated as loopback.
func isLoopbackHost(host string) bool {
	if host == "" || strings.HasSuffix(host, ".") {
		return false
	}

	normalized := strings.ToLower(host)
	if normalized == "localhost" {
		return true
	}

	addr, err := netip.ParseAddr(normalized)
	if err != nil {
		return false
	}

	return addr.IsLoopback()
}
