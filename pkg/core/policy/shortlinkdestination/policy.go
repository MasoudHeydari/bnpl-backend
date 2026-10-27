package shortlinkdestination

import (
	"fmt"
	"net/url"
	"strings"
)

// Policy validates short-link destination URLs against an allowlist of hosts.
type Policy interface {
	// ValidateDestination reports whether destinationURL may be stored.
	ValidateDestination(destinationURL string) error
}

type policy struct {
	allowedHosts map[string]struct{}
}

// New constructs a destination policy from an explicit host allowlist.
//
// Hosts are compared case-insensitively without ports. An empty allowlist
// rejects every destination (useful when the shortener is disabled).
func New(allowedHosts []string) (Policy, error) {
	const op = "shortlinkdestination.New"

	hosts := make(map[string]struct{}, len(allowedHosts))
	for _, host := range allowedHosts {
		normalized := normalizeHost(host)
		if normalized == "" {
			return nil, fmt.Errorf("%s: allowed host must not be empty", op)
		}

		hosts[normalized] = struct{}{}
	}

	return policy{allowedHosts: hosts}, nil
}

func (p policy) ValidateDestination(destinationURL string) error {
	const op = "shortlinkdestination.policy.ValidateDestination"

	raw := strings.TrimSpace(destinationURL)
	if raw == "" {
		return fmt.Errorf("%s: destination url is required", op)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s: parse destination url: %w", op, err)
	}

	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("%s: destination url scheme must be https", op)
	}

	if parsed.Host == "" {
		return fmt.Errorf("%s: destination url host is required", op)
	}

	if parsed.User != nil {
		return fmt.Errorf("%s: destination url must not include userinfo", op)
	}

	host := normalizeHost(parsed.Hostname())
	if _, ok := p.allowedHosts[host]; !ok {
		return fmt.Errorf("%s: destination host %q is not allowed", op, host)
	}

	return nil
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
