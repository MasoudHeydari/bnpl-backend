package sentry

import (
	"context"
	"strings"

	sentrygo "github.com/getsentry/sentry-go"

	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
)

// Config configures the Sentry error-reporting adapter.
type Config struct {
	DSN         string
	Environment string
	Release     string
	SampleRate  float64
	Debug       bool
}

// Reporter sends unexpected failures through a dedicated Sentry client.
//
// A dedicated client is used instead of sentry.Init so the adapter does not
// mutate the SDK's process-global hub and remains explicit in bootstrap wiring.
type Reporter struct {
	client *sentrygo.Client
}

// New constructs a Sentry reporter.
func New(cfg Config) (*Reporter, error) {
	client, err := sentrygo.NewClient(sentrygo.ClientOptions{
		Dsn:              strings.TrimSpace(cfg.DSN),
		Environment:      strings.TrimSpace(cfg.Environment),
		Release:          strings.TrimSpace(cfg.Release),
		SampleRate:       cfg.SampleRate,
		Debug:            cfg.Debug,
		AttachStacktrace: true,

		// Error reporting is intentionally independent from tracing. Sentry
		// Logs/Metrics no longer expose DisableLogs/DisableMetrics (v0.49+);
		// they are opt-in via SDK APIs, which this adapter does not call.
		// Distributed tracing can be added later through OpenTelemetry without
		// coupling the application to Sentry-specific tracing APIs.
		EnableTracing: false,

		DataCollection: &sentrygo.DataCollection{
			UserInfo: sentrygo.Set(false),
			Cookies: &sentrygo.KeyValueCollectionBehavior{
				Mode: sentrygo.CollectionOff,
			},
			HTTPHeaders: &sentrygo.HeaderCollectionConfig{
				Request: &sentrygo.KeyValueCollectionBehavior{
					Mode: sentrygo.CollectionOff,
				},
				Response: &sentrygo.KeyValueCollectionBehavior{
					Mode: sentrygo.CollectionOff,
				},
			},
			// Non-nil empty body configuration explicitly disables request and
			// response body collection.
			HTTPBodies: []sentrygo.BodyType{},
			QueryParams: &sentrygo.KeyValueCollectionBehavior{
				Mode: sentrygo.CollectionOff,
			},
		},
		BeforeSend: sanitizeEvent,
	})
	if err != nil {
		return nil, err
	}

	return &Reporter{client: client}, nil
}

// CaptureError reports an unexpected error together with bounded metadata.
func (r *Reporter) CaptureError(
	ctx context.Context,
	err error,
	metadata coreerrorreporting.Metadata,
) {
	// Error reporting is best-effort and must never become a new application
	// failure, including if the third-party SDK itself panics unexpectedly.
	defer swallowSDKPanic()

	if r == nil || r.client == nil || err == nil {
		return
	}

	ctx = nonNilContext(ctx)
	scope := scopeFromMetadata(metadata)
	r.client.CaptureException(
		err,
		&sentrygo.EventHint{
			Context:           ctx,
			OriginalException: err,
		},
		scope,
	)
}

// CapturePanic records a panic that has already been recovered by a runtime
// boundary. The adapter only reports it and never re-panics.
func (r *Reporter) CapturePanic(
	ctx context.Context,
	recovered any,
	metadata coreerrorreporting.Metadata,
) {
	// This method is commonly called from a deferred recovery path. Never allow
	// an SDK panic to replace the application panic that is being contained.
	defer swallowSDKPanic()

	if r == nil || r.client == nil || recovered == nil {
		return
	}

	ctx = nonNilContext(ctx)
	scope := scopeFromMetadata(metadata)
	r.client.RecoverWithContext(
		ctx,
		recovered,
		&sentrygo.EventHint{Context: ctx},
		scope,
	)
}

// Flush waits for buffered events using the caller's shutdown budget.
func (r *Reporter) Flush(ctx context.Context) (ok bool) {
	if r == nil || r.client == nil {
		return true
	}

	ok = false
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	return r.client.FlushWithContext(nonNilContext(ctx))
}

// Close releases the SDK transport. Transport failures are intentionally not
// surfaced as application shutdown failures.
func (r *Reporter) Close() (err error) {
	if r == nil || r.client == nil {
		return nil
	}

	defer func() {
		if recover() != nil {
			// Observability shutdown must never fail application shutdown.
			err = nil
		}
	}()

	r.client.Close()
	return nil
}

func swallowSDKPanic() {
	_ = recover()
}

func scopeFromMetadata(metadata coreerrorreporting.Metadata) *sentrygo.Scope {
	scope := sentrygo.NewScope()

	if metadata.Component != "" {
		scope.SetTag("component", metadata.Component)
	}

	for key, value := range metadata.Tags {
		if key == "" || sensitiveKey(key) {
			continue
		}

		scope.SetTag(key, value)
	}

	for name, values := range metadata.Contexts {
		if name == "" || sensitiveKey(name) {
			continue
		}

		scope.SetContext(name, sanitizeMap(values))
	}

	if len(metadata.Fingerprint) != 0 {
		scope.SetFingerprint(append([]string(nil), metadata.Fingerprint...))
	}

	return scope
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}
