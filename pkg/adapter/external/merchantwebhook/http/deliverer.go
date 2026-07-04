package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"time"

	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

// Defaults applied when Config leaves a limit unset.
const (
	// DefaultTimeout bounds one whole delivery attempt after a bulkhead slot is
	// acquired (connect, TLS, write, read).
	DefaultTimeout = 10 * time.Second

	// DefaultDialTimeout bounds establishing one TCP connection.
	DefaultDialTimeout = 5 * time.Second

	// DefaultMaxResponseBytes bounds how much of a response body is read.
	DefaultMaxResponseBytes int64 = 64 << 10

	// secureProfileName identifies the webhook secure profile on the shared
	// outbound HTTP client.
	secureProfileName = "merchant_webhook"
)

// Config configures a Deliverer.
type Config struct {
	// Policy is enforced against every resolved and dialed address by the
	// shared HTTP infrastructure secure profile.
	Policy webhookpolicy.Policy

	// Timeout bounds a whole attempt after a bulkhead slot is acquired.
	// Zero uses DefaultTimeout.
	Timeout time.Duration

	// DialTimeout bounds establishing one TCP connection.
	// Zero uses DefaultDialTimeout.
	DialTimeout time.Duration

	// MaxResponseBytes bounds how many response body bytes are read before the
	// body is discarded. Zero uses DefaultMaxResponseBytes.
	MaxResponseBytes int64

	// Executor is the shared outbound HTTP client. Required: webhook delivery
	// never builds a private http.Client or Transport.
	Executor corehttpclient.SecureProfileExecutor
}

// Deliverer is the HTTP implementation of webhook.Deliverer. It owns webhook
// request construction, response classification, and body bounds. Dialing,
// DNS validation, TLS, timeouts, bulkhead, and transport lifecycle are owned
// by the shared HTTP infrastructure profile.
type Deliverer struct {
	executor         corehttpclient.SecureProfileExecutor
	profile          corehttpclient.SecureProfile
	maxResponseBytes int64
}

var _ webhook.Deliverer = (*Deliverer)(nil)

// New constructs a Deliverer and registers its secure HTTP profile on Executor.
func New(cfg Config) (*Deliverer, error) {
	const op = "merchantwebhookhttp.New"

	if cfg.Executor == nil {
		return nil, fmt.Errorf("%s: secure profile executor is required", op)
	}

	if cfg.Timeout < 0 || cfg.DialTimeout < 0 || cfg.MaxResponseBytes < 0 {
		return nil, fmt.Errorf("%s: limits must not be negative", op)
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = DefaultDialTimeout
	}

	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = DefaultMaxResponseBytes
	}

	profile, err := cfg.Executor.NewSecureProfile(corehttpclient.SecureProfileConfig{
		Name:                  secureProfileName,
		NetworkPolicy:         cfg.Policy,
		Timeout:               cfg.Timeout,
		DialTimeout:           cfg.DialTimeout,
		TLSHandshakeTimeout:   cfg.DialTimeout,
		ResponseHeaderTimeout: cfg.Timeout,
		DisableKeepAlives:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: create secure http profile: %w", op, err)
	}

	return &Deliverer{
		executor:         cfg.Executor,
		profile:          profile,
		maxResponseBytes: cfg.MaxResponseBytes,
	}, nil
}

// Close releases no resources. The shared HTTP client owns transport lifecycle;
// External must not Close the infrastructure client through the Deliverer.
func (d *Deliverer) Close() error {
	return nil
}

// Deliver performs one POST attempt and classifies the outcome per the
// webhook.Deliverer contract.
func (d *Deliverer) Deliver(ctx context.Context, delivery webhook.Delivery) error {
	const op = "merchantwebhookhttp.Deliverer.Deliver"

	if delivery.URL == nil {
		return messaging.NonRetryable(fmt.Errorf("%s: url is required", op))
	}

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, delivery.URL.String(), bytes.NewReader(delivery.Body))
	if err != nil {
		return messaging.NonRetryable(fmt.Errorf("%s: build request", op))
	}

	for name, value := range delivery.Headers {
		req.Header.Set(name, value)
	}

	resp, err := d.executor.DoWithSecureProfile(req, d.profile)
	if err != nil {
		return classifyTransportError(op, err)
	}

	// Drain a bounded amount so the connection closes cleanly, then discard.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, d.maxResponseBytes))
	_ = resp.Body.Close()

	return classifyStatus(op, resp.StatusCode)
}

func classifyStatus(op string, status int) error {
	switch {
	case status >= 200 && status < 300:
		return nil

	case status == nethttp.StatusRequestTimeout, status == nethttp.StatusTooManyRequests, status >= 500:
		return fmt.Errorf("%s: endpoint responded with retryable status %d", op, status)

	case status >= 300 && status < 400:
		return messaging.NonRetryable(fmt.Errorf("%s: endpoint redirected (status %d); redirects are not followed", op, status))

	default:
		return messaging.NonRetryable(fmt.Errorf("%s: endpoint rejected request with status %d", op, status))
	}
}

// classifyTransportError maps a client error to retryable or permanent without
// echoing the request URL (url.Error text) into persisted outbox state.
func classifyTransportError(op string, err error) error {
	switch {
	case errors.Is(err, webhookpolicy.ErrBlockedAddress):
		return messaging.NonRetryable(fmt.Errorf("%s: %w", op, webhookpolicy.ErrBlockedAddress))

	case errors.Is(err, webhookpolicy.ErrBlockedHost):
		return messaging.NonRetryable(fmt.Errorf("%s: %w", op, webhookpolicy.ErrBlockedHost))

	case errors.Is(err, corehttpclient.ErrDisabled):
		return messaging.NonRetryable(fmt.Errorf("%s: outbound http client is disabled", op))

	case errors.Is(err, corehttpclient.ErrSaturated), errors.Is(err, corehttpclient.ErrQueueWaitTimeout):
		return fmt.Errorf("%s: outbound http client capacity exhausted", op)

	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s: request timed out", op)

	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s: request canceled", op)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return fmt.Errorf("%s: request timed out", op)
	}

	cause := err
	if urlErr != nil && urlErr.Err != nil {
		cause = urlErr.Err
	}

	return fmt.Errorf("%s: network error: %s", op, cause.Error())
}
