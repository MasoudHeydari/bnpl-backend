package webhook

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// URL policy errors. They describe a destination that can never be delivered to
// under the active policy, so callers treat them as permanent.
var (
	// ErrInvalidURL means the URL is malformed or violates structural rules.
	ErrInvalidURL = errors.New("invalid webhook url")

	// ErrInsecureScheme means HTTPS is required but the URL uses another scheme.
	ErrInsecureScheme = errors.New("webhook url must use https")

	// ErrBlockedHost means the hostname is forbidden (localhost, internal names,
	// metadata endpoints, numeric hosts).
	ErrBlockedHost = errors.New("webhook host is not allowed")

	// ErrBlockedAddress means an IP address (literal or resolved) is forbidden.
	ErrBlockedAddress = errors.New("webhook address is not allowed")
)

// Policy controls which webhook destinations may be contacted.
//
// DNS lookup and dial-time rebinding checks are performed by the delivery
// adapter using these same rules against resolved addresses.
type Policy struct {
	// RequireHTTPS rejects plain http URLs.
	RequireHTTPS bool

	// AllowLoopback permits loopback hosts and addresses (localhost, 127.0.0.0/8,
	// ::1). Every other private, link-local, and metadata range stays blocked.
	AllowLoopback bool
}

// Strict returns the production/staging destination policy: HTTPS only, no
// loopback or private targets.
func Strict() Policy {
	return Policy{RequireHTTPS: true, AllowLoopback: false}
}

// Development returns the local-development destination policy: HTTP and
// loopback are allowed so local receivers can be used. Private, link-local, and
// metadata ranges stay blocked.
func Development() Policy {
	return Policy{RequireHTTPS: false, AllowLoopback: true}
}

// blockedPrefixes lists non-routable or sensitive ranges that are never
// permitted, in addition to the netip classification helpers.
var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // "this network"
	"100.64.0.0/10",   // carrier-grade NAT (includes Alibaba metadata 100.100.100.200)
	"169.254.0.0/16",  // link-local (cloud metadata 169.254.169.254)
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // TEST-NET-1
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"240.0.0.0/4",     // reserved + broadcast
	"fc00::/7",        // unique local (includes AWS IPv6 metadata fd00:ec2::254)
	"fe80::/10",       // link-local
	"64:ff9b::/96",    // NAT64 can embed internal IPv4 targets
	"2002::/16",       // 6to4 can embed internal IPv4 targets
	"100::/64",        // discard-only
)

func mustPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefixes = append(prefixes, netip.MustParsePrefix(value))
	}

	return prefixes
}

// blockedHostnames are exact names that are never contacted.
var blockedHostnames = map[string]struct{}{
	"metadata":                 {},
	"metadata.google.internal": {},
	"instance-data":            {},
	"kubernetes":               {},
	"kubernetes.default":       {},
}

// blockedHostSuffixes are internal-only DNS suffixes.
var blockedHostSuffixes = []string{
	".internal",
	".local",
	".localdomain",
	".lan",
	".home",
	".corp",
	".intranet",
	".cluster.local",
	".svc",
}

// IsBlockedAddr reports whether addr may not be contacted under p.
//
// IPv4-mapped IPv6 addresses are unmapped first so ::ffff:127.0.0.1 is judged as
// 127.0.0.1.
func (p Policy) IsBlockedAddr(addr netip.Addr) bool {
	addr = addr.Unmap()

	if !addr.IsValid() || addr.Zone() != "" {
		return true
	}

	if addr.IsLoopback() {
		return !p.AllowLoopback
	}

	if addr.IsUnspecified() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() {
		return true
	}

	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

// CheckAddr returns ErrBlockedAddress when addr may not be contacted.
func (p Policy) CheckAddr(addr netip.Addr) error {
	if p.IsBlockedAddr(addr) {
		return ErrBlockedAddress
	}

	return nil
}

// CheckHost validates a URL hostname (no port, no brackets). IP literals are
// judged by address; names are screened for loopback, internal-only, metadata,
// single-label and numeric forms. Resolved addresses must still be checked at
// dial time with CheckAddr.
func (p Policy) CheckHost(host string) error {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return fmt.Errorf("%w: host is empty", ErrInvalidURL)
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		return p.CheckAddr(addr)
	}

	if strings.ContainsAny(host, "%[]") {
		return ErrBlockedHost
	}

	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		if p.AllowLoopback {
			return nil
		}

		return ErrBlockedHost
	}

	if _, blocked := blockedHostnames[host]; blocked {
		return ErrBlockedHost
	}

	if p.AllowLoopback && !strings.Contains(host, ".") {
		// Development hostnames such as docker service names are still subject to
		// resolved-address checks at dial time.
		return nil
	}

	for _, suffix := range blockedHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return ErrBlockedHost
		}
	}

	// Single-label names reach internal resolvers (service names, search domains).
	if !strings.Contains(host, ".") {
		return ErrBlockedHost
	}

	// A numeric final label cannot be a real TLD; it is an obfuscated IP form
	// such as 2130706433 or 0x7f.1 that some resolvers expand to loopback.
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]

	if _, err := strconv.ParseUint(last, 0, 64); err == nil {
		return ErrBlockedHost
	}

	return nil
}

// ValidateURL parses and validates a merchant webhook URL under p.
//
// The URL must be absolute http/https with a host, no userinfo, and no fragment.
// HTTPS is required when p.RequireHTTPS is set. Hostname and literal-IP rules are
// applied here; hostnames must additionally have their resolved addresses
// checked at connection time.
func (p Policy) ValidateURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) != raw || raw == "" {
		return nil, fmt.Errorf("%w: url must be non-empty without surrounding whitespace", ErrInvalidURL)
	}

	if strings.ContainsRune(raw, '#') {
		return nil, fmt.Errorf("%w: fragment is not allowed", ErrInvalidURL)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed url", ErrInvalidURL)
	}

	// Empty "?" is still a query delimiter (ForceQuery with empty RawQuery).
	// Detect via the parsed URL — do not use strings.HasSuffix(raw, "?"), which
	// wrongly rejects non-empty queries whose value ends with '?' (e.g. ?token=abc?).
	if parsed.ForceQuery && parsed.RawQuery == "" {
		return nil, fmt.Errorf("%w: empty query delimiter is not allowed", ErrInvalidURL)
	}

	if !parsed.IsAbs() || parsed.Opaque != "" {
		return nil, fmt.Errorf("%w: url must be absolute", ErrInvalidURL)
	}

	switch parsed.Scheme {
	case "https":
		// Always permitted.

	case "http":
		if p.RequireHTTPS {
			return nil, ErrInsecureScheme
		}

	default:
		return nil, fmt.Errorf("%w: scheme must be http or https", ErrInvalidURL)
	}

	if parsed.User != nil {
		return nil, fmt.Errorf("%w: userinfo is not allowed", ErrInvalidURL)
	}

	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("%w: host is required", ErrInvalidURL)
	}

	if port := parsed.Port(); port != "" {
		number, portErr := strconv.Atoi(port)
		if portErr != nil || number < 1 || number > 65535 {
			return nil, fmt.Errorf("%w: invalid port", ErrInvalidURL)
		}
	}

	if hostErr := p.CheckHost(parsed.Hostname()); hostErr != nil {
		return nil, hostErr
	}

	return parsed, nil
}
