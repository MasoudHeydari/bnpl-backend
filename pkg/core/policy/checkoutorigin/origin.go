package checkoutorigin

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// Policy errors describe an origin that must never be used to build hosted
// checkout URLs. Callers treat them as permanent configuration failures.
var (
	// ErrOriginEmpty means the public origin is missing.
	ErrOriginEmpty = errors.New("checkout public origin is required")

	// ErrOriginInvalid means the value is not a strict origin
	// (scheme://host[:port] with no path/query/fragment/userinfo/whitespace).
	ErrOriginInvalid = errors.New("checkout public origin is invalid")

	// ErrInsecureScheme means HTTPS is required or HTTP was used with a
	// non-loopback host under AllowLoopbackHTTP.
	ErrInsecureScheme = errors.New("checkout public origin must use https")

	// ErrHostNotAllowed means the hostname is outside the configured allowlist
	// (or is loopback/IP when those are rejected).
	ErrHostNotAllowed = errors.New("checkout public origin host is not allowed")

	// ErrPortNotAllowed means the origin port is forbidden under the active policy.
	ErrPortNotAllowed = errors.New("checkout public origin port is not allowed")

	// ErrAllowedHostsRequired means production-style policy was built without hosts.
	ErrAllowedHostsRequired = errors.New("checkout public origin allowed hosts are required")
)

// Policy controls scheme, host, and port rules for the hosted checkout public
// origin.
//
// Structural origin rules always apply. Policy selection is performed by
// Config/Bootstrap from the runtime environment; this type stays pure.
//
// The checkout public origin is trusted deployment configuration because
// generated checkout URLs are returned to merchants and browsers. Production
// host pinning (AllowedHosts) prevents valid-but-incorrect environment values
// from redirecting customers to an unintended origin.
//
// Callers should construct policies via Strict / Development rather than
// assembling boolean fields by hand.
type Policy struct {
	requireHTTPS               bool
	allowLoopbackHTTP          bool
	rejectLoopback             bool
	rejectIPLiterals           bool
	restrictHTTPSPortToDefault bool
	allowedHosts               []string
}

// Strict returns the production/staging origin policy: HTTPS only, no
// loopback or IP literals, default HTTPS port only, and an explicit host
// allowlist. allowedHosts must be non-empty and independent of the origin
// value under validation.
func Strict(allowedHosts []string) Policy {
	return Policy{
		requireHTTPS:               true,
		allowLoopbackHTTP:          false,
		rejectLoopback:             true,
		rejectIPLiterals:           true,
		restrictHTTPSPortToDefault: true,
		allowedHosts:               cloneHosts(allowedHosts),
	}
}

// Development returns the local-development / test origin policy.
//
// HTTPS is always allowed for allowlisted hosts. HTTP is allowed only for
// localhost / loopback (for example http://localhost:3000). Non-loopback IP
// literals and plain http to public hostnames are rejected.
func Development(allowedHosts []string) Policy {
	return Policy{
		requireHTTPS:               false,
		allowLoopbackHTTP:          true,
		rejectLoopback:             false,
		rejectIPLiterals:           true,
		restrictHTTPSPortToDefault: false,
		allowedHosts:               cloneHosts(allowedHosts),
	}
}

func cloneHosts(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}

	out := make([]string, len(hosts))
	copy(out, hosts)

	return out
}

// PublicOrigin is a validated hosted-checkout origin (scheme://host[:port]).
//
// It intentionally excludes paths, query strings, and fragments. Construction
// is only possible through Policy.Parse.
type PublicOrigin struct {
	raw string
}

// String returns the canonical origin string.
func (o PublicOrigin) String() string {
	return o.raw
}

// IsZero reports whether o is the zero PublicOrigin.
func (o PublicOrigin) IsZero() bool {
	return o.raw == ""
}

// Parse validates raw as a strict public origin under p.
//
// Rules:
//   - required; leading/trailing whitespace and control characters are rejected
//     (not silently trimmed)
//   - absolute http(s) URL that is exactly an origin: no path, query, fragment,
//     userinfo, or opaque part
//   - empty query / fragment delimiters ("?", "#") are rejected — misconfiguration
//     is never silently stripped
//   - trailing slash is rejected (path must be empty, not "/")
//   - trailing-dot hostnames are rejected (not TrimSuffix-normalized into the
//     allowlist)
//   - https always accepted when host/port policy allows
//   - http accepted only when allowLoopbackHTTP and host is loopback
//   - non-loopback hostnames must exactly match AllowedHosts (case-insensitive)
func (p Policy) Parse(raw string) (PublicOrigin, error) {
	if raw == "" {
		return PublicOrigin{}, ErrOriginEmpty
	}

	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\t") {
		return PublicOrigin{}, ErrOriginInvalid
	}

	// Reject query/fragment delimiters before parsing so empty "?" / "#" cannot
	// be normalized away (ForceQuery / empty Fragment).
	if strings.ContainsAny(raw, "?#") {
		return PublicOrigin{}, ErrOriginInvalid
	}

	if p.requireHTTPS || p.rejectLoopback || p.rejectIPLiterals {
		if len(p.allowedHosts) == 0 {
			return PublicOrigin{}, ErrAllowedHostsRequired
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return PublicOrigin{}, ErrOriginInvalid
	}

	if parsed.Opaque != "" || parsed.User != nil || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawQuery != "" {
		return PublicOrigin{}, ErrOriginInvalid
	}

	// Origins must not carry a path. url.Parse("https://h/") yields Path="/".
	if parsed.Path != "" || parsed.RawPath != "" {
		return PublicOrigin{}, ErrOriginInvalid
	}

	hostname := parsed.Hostname()
	if hostname == "" || parsed.Scheme == "" || parsed.Host == "" {
		return PublicOrigin{}, ErrOriginInvalid
	}

	// Trailing-dot FQDNs are not silently equivalent to bare hostnames.
	if strings.HasSuffix(hostname, ".") {
		return PublicOrigin{}, ErrOriginInvalid
	}

	normalizedHost := strings.ToLower(hostname)
	if normalizedHost == "" {
		return PublicOrigin{}, ErrOriginInvalid
	}

	port := parsed.Port()
	loopback := isLoopbackHost(normalizedHost)
	ipLiteral := isIPLiteral(normalizedHost)

	switch parsed.Scheme {
	case "https":
		if p.restrictHTTPSPortToDefault && port != "" && port != "443" {
			return PublicOrigin{}, ErrPortNotAllowed
		}

	case "http":
		if p.requireHTTPS {
			return PublicOrigin{}, ErrInsecureScheme
		}

		if !p.allowLoopbackHTTP || !loopback {
			return PublicOrigin{}, ErrInsecureScheme
		}

	default:
		return PublicOrigin{}, ErrOriginInvalid
	}

	if loopback {
		if p.rejectLoopback {
			return PublicOrigin{}, ErrHostNotAllowed
		}
	} else if ipLiteral {
		if p.rejectIPLiterals {
			return PublicOrigin{}, ErrHostNotAllowed
		}

		if !hostAllowed(normalizedHost, p.allowedHosts) {
			return PublicOrigin{}, ErrHostNotAllowed
		}
	} else if !hostAllowed(normalizedHost, p.allowedHosts) {
		return PublicOrigin{}, ErrHostNotAllowed
	}

	canonical := (&url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
	}).String()

	return PublicOrigin{raw: canonical}, nil
}

func hostAllowed(normalizedHost string, allowed []string) bool {
	for _, candidate := range allowed {
		if strings.HasSuffix(candidate, ".") {
			continue
		}

		if strings.EqualFold(candidate, normalizedHost) {
			return true
		}
	}

	return false
}

func isLoopbackHost(normalizedHost string) bool {
	if normalizedHost == "" {
		return false
	}

	if normalizedHost == "localhost" {
		return true
	}

	addr, err := netip.ParseAddr(normalizedHost)
	if err != nil {
		return false
	}

	return addr.IsLoopback()
}

func isIPLiteral(normalizedHost string) bool {
	if normalizedHost == "" {
		return false
	}

	if _, err := netip.ParseAddr(normalizedHost); err == nil {
		return true
	}

	return net.ParseIP(normalizedHost) != nil
}
