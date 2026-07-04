package httpclient

import "net/netip"

// NetworkPolicy decides which hostnames and IP addresses may be dialed by a
// secure outbound profile. Pure destination rules belong here or in a policy
// package; the HTTP adapter only enforces the injected policy at resolve/dial
// time.
//
// Default outbound HTTP traffic must not inherit a webhook-specific policy
// unless a secure profile is explicitly constructed with one.
type NetworkPolicy interface {
	// CheckHost rejects forbidden hostnames before DNS lookup.
	CheckHost(host string) error

	// CheckAddr rejects forbidden IP addresses (literals or resolved answers).
	CheckAddr(addr netip.Addr) error
}
