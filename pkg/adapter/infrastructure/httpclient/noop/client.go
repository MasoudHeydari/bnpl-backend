package noop

import (
	"fmt"
	"net/http"
	"strings"

	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

var (
	_ corehttpclient.Client                = (*Client)(nil)
	_ corehttpclient.SecureProfileExecutor = (*Client)(nil)
)

type noopSecureProfile struct {
	name string
}

func (p *noopSecureProfile) IsSecureHTTPProfile() {}

// Client rejects every request with corehttpclient.ErrDisabled.
type Client struct {
	logger   corelog.Logger
	profiles map[string]*noopSecureProfile
}

// New constructs a noop outbound HTTP client.
func New(logger corelog.Logger) (*Client, error) {
	return &Client{
		logger:   logger,
		profiles: make(map[string]*noopSecureProfile),
	}, nil
}

// Do performs no network I/O and never fabricates an external success.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	logger := c.logger
	if logger != nil {
		if req != nil && req.URL != nil {
			logger = logger.With(
				"http_method", req.Method,
				"http_scheme", req.URL.Scheme,
				"http_host", req.URL.Host,
			)
		}

		logger.Warn("outbound http request rejected because noop provider is enabled")
	}

	return nil, corehttpclient.ErrDisabled
}

// Close is a no-op because the noop provider owns no network resources.
func (*Client) Close() error { return nil }

// NewSecureProfile records a secure profile name without performing network I/O.
func (c *Client) NewSecureProfile(cfg corehttpclient.SecureProfileConfig) (corehttpclient.SecureProfile, error) {
	const op = "noop.Client.NewSecureProfile"

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

	if existing, ok := c.profiles[name]; ok {
		return existing, nil
	}

	profile := &noopSecureProfile{name: name}
	c.profiles[name] = profile

	return profile, nil
}

// DoWithSecureProfile rejects every request with ErrDisabled.
func (c *Client) DoWithSecureProfile(req *http.Request, profile corehttpclient.SecureProfile) (*http.Response, error) {
	const op = "noop.Client.DoWithSecureProfile"

	if _, ok := profile.(*noopSecureProfile); !ok || profile == nil {
		return nil, fmt.Errorf("%s: unknown secure profile", op)
	}

	return c.Do(req)
}
