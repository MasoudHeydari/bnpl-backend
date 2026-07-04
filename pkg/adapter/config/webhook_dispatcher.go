package config

import (
	"fmt"
	"io"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook"
	merchantwebhookhttp "github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook/http"
	merchantwebhooknoop "github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook/noop"
	messagingrouter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/router"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

// ExternalWebhookDispatcher configures outbound merchant webhook delivery.
//
// Provider selects the adapter. Provider-specific settings live under nested
// sections (noop / http) so future adapters can add independent configuration.
type ExternalWebhookDispatcher struct {
	// Provider selects the webhook delivery adapter: "http" or "noop".
	Provider string `yaml:"provider"`

	// Noop holds noop provider settings.
	Noop ExternalWebhookDispatcherNoop `yaml:"noop"`

	// HTTP holds HTTP provider settings.
	HTTP ExternalWebhookDispatcherHTTP `yaml:"http"`
}

// ExternalWebhookDispatcherNoop holds noop provider settings.
type ExternalWebhookDispatcherNoop struct{}

// ExternalWebhookDispatcherHTTP holds HTTP provider delivery limits.
type ExternalWebhookDispatcherHTTP struct {
	// Timeout bounds one whole delivery attempt.
	Timeout time.Duration `yaml:"timeout"`

	// DialTimeout bounds establishing one TCP connection.
	DialTimeout time.Duration `yaml:"dial_timeout"`

	// MaxResponseBytes bounds how much of a response body is read.
	MaxResponseBytes int64 `yaml:"max_response_bytes"`
}

// ApplyDefaults sets provider to http when unset so local configs can omit the
// field without accidentally selecting noop, and fills HTTP delivery defaults
// used when the http provider is selected.
func (w *ExternalWebhookDispatcher) ApplyDefaults() {
	if w.Provider == "" {
		w.Provider = string(merchantwebhook.HTTPProvider)
	}

	if w.HTTP.Timeout == 0 {
		w.HTTP.Timeout = merchantwebhookhttp.DefaultTimeout
	}

	if w.HTTP.DialTimeout == 0 {
		w.HTTP.DialTimeout = merchantwebhookhttp.DefaultDialTimeout
	}

	if w.HTTP.MaxResponseBytes == 0 {
		w.HTTP.MaxResponseBytes = merchantwebhookhttp.DefaultMaxResponseBytes
	}
}

func (w ExternalWebhookDispatcher) validate(env model.RuntimeEnvironment) error {
	const op = "config.ExternalWebhookDispatcher.validate"

	switch merchantwebhook.Provider(w.Provider) {
	case merchantwebhook.HTTPProvider:
		// Defaults are applied before validate so zero means "use defaults"
		// rather than "unset and invalid".
		if err := w.HTTP.validate(); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

	case merchantwebhook.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: external.webhook_dispatcher provider \"noop\" is not allowed in production", op)
		}

	default:
		return fmt.Errorf("%s: invalid external.webhook_dispatcher.provider: unsupported value %q", op, w.Provider)
	}

	return nil
}

func (h ExternalWebhookDispatcherHTTP) validate() error {
	const op = "config.ExternalWebhookDispatcherHTTP.validate"

	if h.Timeout < 0 {
		return fmt.Errorf("%s: invalid external.webhook_dispatcher.http.timeout: must not be negative", op)
	}

	if h.DialTimeout < 0 {
		return fmt.Errorf("%s: invalid external.webhook_dispatcher.http.dial_timeout: must not be negative", op)
	}

	if h.MaxResponseBytes < 0 {
		return fmt.Errorf("%s: invalid external.webhook_dispatcher.http.max_response_bytes: must not be negative", op)
	}

	return nil
}

// newRoutingPublisher wraps fallback (Kafka or noop messaging) so outbox rows
// addressed to webhook://merchant/<id> are handled by the configured webhook
// provider while every other destination keeps using fallback.
//
// The optional closer owns HTTP deliverer resources. The returned router does
// not close the messaging fallback; Infrastructure owns that publisher.
func (w ExternalWebhookDispatcher) newRoutingPublisher(
	checkoutSessionCfg CheckoutSession,
	env model.RuntimeEnvironment,
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	fallback messaging.Publisher,
	clk clock.Clock,
	logger corelog.Logger,
	businessMetrics metrics.BusinessMetrics,
	httpClient corehttpclient.Client,
) (messaging.Publisher, io.Closer, error) {
	const op = "config.ExternalWebhookDispatcher.newRoutingPublisher"

	webhookPublisher, closer, err := w.newPublisher(
		checkoutSessionCfg, env, pool, checkoutSession, clk, logger, businessMetrics, httpClient,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: create webhook publisher: %w", op, err)
	}

	router, err := messagingrouter.New(fallback, messagingrouter.Route{
		Prefix:    webhook.DestinationPrefix,
		Publisher: webhookPublisher,
	})
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}

		return nil, nil, fmt.Errorf("%s: create publisher router: %w", op, err)
	}

	return router, closer, nil
}

func (w ExternalWebhookDispatcher) newPublisher(
	checkoutSessionCfg CheckoutSession,
	env model.RuntimeEnvironment,
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	clk clock.Clock,
	logger corelog.Logger,
	businessMetrics metrics.BusinessMetrics,
	httpClient corehttpclient.Client,
) (messaging.Publisher, io.Closer, error) {
	const op = "config.ExternalWebhookDispatcher.newPublisher"

	switch merchantwebhook.Provider(w.Provider) {
	case merchantwebhook.NoopProvider:
		publisher, err := merchantwebhooknoop.NewPublisher(logger, businessMetrics)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: create noop webhook publisher: %w", op, err)
		}

		return publisher, nil, nil

	case merchantwebhook.HTTPProvider, "":
		return w.newHTTPPublisher(
			checkoutSessionCfg, env, pool, checkoutSession, clk, logger, businessMetrics, httpClient,
		)

	default:
		return nil, nil, fmt.Errorf("%s: unsupported external.webhook_dispatcher.provider %q", op, w.Provider)
	}
}

func (w ExternalWebhookDispatcher) newHTTPPublisher(
	checkoutSessionCfg CheckoutSession,
	env model.RuntimeEnvironment,
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	clk clock.Clock,
	logger corelog.Logger,
	businessMetrics metrics.BusinessMetrics,
	httpClient corehttpclient.Client,
) (messaging.Publisher, io.Closer, error) {
	const op = "config.ExternalWebhookDispatcher.newHTTPPublisher"

	box, err := checkoutSessionCfg.NewWebhookSecretBox()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: create webhook secret box: %w", op, err)
	}

	policy := webhookPolicyForEnvironment(env)

	if httpClient == nil {
		return nil, nil, fmt.Errorf("%s: http client is required for webhook delivery", op)
	}

	executor, ok := httpClient.(corehttpclient.SecureProfileExecutor)
	if !ok {
		return nil, nil, fmt.Errorf("%s: http client must implement SecureProfileExecutor for SSRF-safe webhook delivery", op)
	}

	deliverer, err := merchantwebhookhttp.New(merchantwebhookhttp.Config{
		Policy:           policy,
		Timeout:          w.HTTP.Timeout,
		DialTimeout:      w.HTTP.DialTimeout,
		MaxResponseBytes: w.HTTP.MaxResponseBytes,
		Executor:         executor,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%s: create webhook deliverer: %w", op, err)
	}

	dispatcher, err := merchantwebhookhttp.NewDispatcher(
		pool, checkoutSession, box, deliverer, policy, clk, logger, businessMetrics,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: create webhook dispatcher: %w", op, err)
	}

	// Deliverer owns no transport; the shared infrastructure HTTP client does.
	return dispatcher, nil, nil
}

// webhookPolicyForEnvironment maps runtime environment onto a
// pkg/core/policy/webhook profile. Development and test allow loopback HTTP for
// local receivers; every other environment, including unknown values, is strict.
func webhookPolicyForEnvironment(env model.RuntimeEnvironment) webhookpolicy.Policy {
	switch env {
	case model.DevelopmentRuntimeEnvironment, model.TestRuntimeEnvironment:
		return webhookpolicy.Development()

	default:
		return webhookpolicy.Strict()
	}
}
