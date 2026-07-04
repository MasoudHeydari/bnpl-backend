package standard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"syscall"

	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
)

// Resolver resolves hostnames to IP addresses for secure dialing tests and
// production DNS lookups.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type secureProfile struct {
	name       string
	client     *http.Client
	transport  *http.Transport
	policy     corehttpclient.NetworkPolicy
	config     corehttpclient.SecureProfileConfig
	resolverMu sync.RWMutex
	resolver   Resolver
	dialer     *net.Dialer
}

func (p *secureProfile) IsSecureHTTPProfile() {}

var _ corehttpclient.SecureProfile = (*secureProfile)(nil)

// NewSecureProfile builds or returns a reusable secure outbound profile that
// shares this client's bulkhead and metrics. Network I/O does not occur here.
func (c *Client) NewSecureProfile(cfg corehttpclient.SecureProfileConfig) (corehttpclient.SecureProfile, error) {
	const op = "standard.Client.NewSecureProfile"

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return nil, fmt.Errorf("%s: profile name is required", op)
	}

	if cfg.NetworkPolicy == nil {
		return nil, fmt.Errorf("%s: network policy is required", op)
	}

	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("%s: timeout must be positive", op)
	}

	if cfg.DialTimeout <= 0 {
		return nil, fmt.Errorf("%s: dial timeout must be positive", op)
	}

	tlsHandshakeTimeout := cfg.TLSHandshakeTimeout
	if tlsHandshakeTimeout <= 0 {
		tlsHandshakeTimeout = cfg.DialTimeout
	}

	responseHeaderTimeout := cfg.ResponseHeaderTimeout
	if responseHeaderTimeout <= 0 {
		responseHeaderTimeout = cfg.Timeout
	}

	c.profilesMu.Lock()
	defer c.profilesMu.Unlock()

	// A profile name is an immutable configuration identity, not a cache key
	// that silently tolerates changed timeout or security policy settings.
	// Comparing the normalized configuration catches accidental reuse with
	// weaker policy or different deadlines during startup.
	cfg.Name = name
	cfg.TLSHandshakeTimeout = tlsHandshakeTimeout
	cfg.ResponseHeaderTimeout = responseHeaderTimeout
	if existing, ok := c.profiles[name]; ok {
		if !reflect.DeepEqual(existing.config, cfg) {
			return nil, fmt.Errorf("%s: secure profile %q already exists with different configuration", op, name)
		}
		return existing, nil
	}

	profile := &secureProfile{
		name:     name,
		policy:   cfg.NetworkPolicy,
		config:   cfg,
		resolver: net.DefaultResolver,
	}

	profile.dialer = &net.Dialer{
		Timeout: cfg.DialTimeout,
		// Second layer: validate the address the socket is about to connect to.
		Control: profile.controlAddress,
	}

	profile.transport = &http.Transport{
		// Never honor HTTP(S)_PROXY: a proxy would connect on our behalf and
		// bypass address validation.
		Proxy:                 nil,
		DialContext:           profile.dialContext,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
		DisableKeepAlives:     cfg.DisableKeepAlives,
		ForceAttemptHTTP2:     true,
	}

	profile.client = &http.Client{
		Transport: profile.transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	c.profiles[name] = profile

	c.logger.With(
		"profile", name,
		"request_timeout", cfg.Timeout.String(),
		"dial_timeout", cfg.DialTimeout.String(),
	).Info("outbound http secure profile initialized")

	return profile, nil
}

// SetSecureProfileResolver replaces the DNS resolver used by a secure profile.
// Intended for tests; production code leaves the default resolver in place.
func (c *Client) SetSecureProfileResolver(profile corehttpclient.SecureProfile, resolver Resolver) error {
	const op = "standard.Client.SetSecureProfileResolver"

	sp, ok := profile.(*secureProfile)
	if !ok || sp == nil {
		return fmt.Errorf("%s: unknown secure profile", op)
	}

	if resolver == nil {
		return fmt.Errorf("%s: resolver is required", op)
	}
	c.profilesMu.RLock()
	owned := c.profiles[sp.name] == sp
	c.profilesMu.RUnlock()
	if !owned {
		return fmt.Errorf("%s: profile belongs to another client", op)
	}

	// Resolver replacement is a test seam, and also must remain safe under
	// concurrent request execution (including -race).
	sp.resolverMu.Lock()
	sp.resolver = resolver
	sp.resolverMu.Unlock()

	return nil
}

// DoWithSecureProfile executes req through the shared bulkhead using the
// profile's prebuilt client and timeout. No per-request http.Client is created.
func (c *Client) DoWithSecureProfile(
	req *http.Request,
	profile corehttpclient.SecureProfile,
) (*http.Response, error) {
	const op = "standard.Client.DoWithSecureProfile"

	sp, ok := profile.(*secureProfile)
	if !ok || sp == nil {
		return nil, fmt.Errorf("%s: unknown secure profile", op)
	}

	c.profilesMu.RLock()
	owned := c.profiles[sp.name] == sp
	c.profilesMu.RUnlock()
	if !owned {
		return nil, fmt.Errorf("%s: profile belongs to another client", op)
	}

	return c.do(req, sp.client.Do)
}

// dialContext resolves the hostname, rejects the connection if any resolved
// address is blocked, and dials a validated IP literal directly so the HTTP
// stack cannot re-resolve the hostname to an unvalidated address (DNS rebinding).
func (p *secureProfile) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("malformed dial address: %w", err)
	}

	addrs, err := p.resolve(ctx, host)
	if err != nil {
		return nil, err
	}

	var lastErr error

	for _, addr := range addrs {
		conn, dialErr := p.dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if dialErr == nil {
			return conn, nil
		}

		if errors.Is(dialErr, context.Canceled) || errors.Is(dialErr, context.DeadlineExceeded) {
			return nil, dialErr
		}

		// Control denials are permanent; do not try remaining addresses.
		if checkErr := p.policy.CheckAddr(addr); checkErr != nil && errors.Is(dialErr, checkErr) {
			return nil, checkErr
		}

		lastErr = dialErr
	}

	return nil, lastErr
}

func (p *secureProfile) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if checkErr := p.policy.CheckAddr(addr); checkErr != nil {
			return nil, checkErr
		}

		return []netip.Addr{addr.Unmap()}, nil
	}

	if err := p.policy.CheckHost(host); err != nil {
		return nil, err
	}

	p.resolverMu.RLock()
	resolver := p.resolver
	p.resolverMu.RUnlock()
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve host: %w", err)
	}

	if len(addrs) == 0 {
		return nil, errors.New("resolve host: no addresses")
	}

	validated := make([]netip.Addr, 0, len(addrs))

	for _, addr := range addrs {
		if checkErr := p.policy.CheckAddr(addr); checkErr != nil {
			return nil, checkErr
		}

		validated = append(validated, addr.Unmap())
	}

	return validated, nil
}

func (p *secureProfile) controlAddress(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("malformed dial address")
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("dial address is not an IP literal")
	}

	return p.policy.CheckAddr(addr)
}

// closeSecureProfiles closes idle connections owned by secure profile transports.
func (c *Client) closeSecureProfiles() {
	c.profilesMu.Lock()
	defer c.profilesMu.Unlock()

	for _, profile := range c.profiles {
		if profile.transport != nil {
			profile.transport.CloseIdleConnections()
		}
	}
}
