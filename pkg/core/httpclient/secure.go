package httpclient

import (
	"net/http"
	"time"
)

// SecureProfile is an opaque handle to a prebuilt secure outbound HTTP profile.
// Profiles are created once at initialization and reused; callers must not
// construct net/http.Client or Transport instances per request.
type SecureProfile interface {
	// IsSecureHTTPProfile marks implementations constructed by a
	// SecureProfileExecutor. External packages must not invent profiles.
	IsSecureHTTPProfile()
}

// SecureProfileConfig configures one reusable secure outbound profile.
//
// Timeout is the end-to-end timeout for requests executed through this profile
// and is independent of the default outbound client timeout. Queue wait time in
// the shared bulkhead is not included in Timeout; request context deadlines
// still apply to both queue wait and execution.
type SecureProfileConfig struct {
	// Name uniquely identifies the profile on the shared client.
	Name string

	// NetworkPolicy is enforced at DNS resolution and dial time.
	NetworkPolicy NetworkPolicy

	// Timeout bounds one whole attempt after an execution slot is acquired
	// (connect, TLS, write, response headers, and body reads started by the
	// client). Zero is invalid.
	Timeout time.Duration

	// DialTimeout bounds establishing one TCP connection. Zero is invalid.
	DialTimeout time.Duration

	// TLSHandshakeTimeout bounds TLS handshake completion. Zero uses DialTimeout.
	TLSHandshakeTimeout time.Duration

	// ResponseHeaderTimeout bounds waiting for response headers after the
	// request is written. Zero uses Timeout.
	ResponseHeaderTimeout time.Duration

	// DisableKeepAlives disables connection reuse for this profile.
	DisableKeepAlives bool
}

// SecureProfileExecutor constructs and executes requests through named secure
// profiles that share the outbound bulkhead and metrics of the host Client.
type SecureProfileExecutor interface {
	// NewSecureProfile builds (or returns) a reusable secure profile. It must
	// not perform network I/O.
	NewSecureProfile(cfg SecureProfileConfig) (SecureProfile, error)

	// DoWithSecureProfile executes req through the shared bulkhead using the
	// profile's prebuilt transport and timeout. Redirects are never followed.
	DoWithSecureProfile(req *http.Request, profile SecureProfile) (*http.Response, error)
}
