package config

import (
	"fmt"
	"time"

	gingonic "github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	i18nbuiltin "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n/builtin"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

const (
	defaultGinReadHeaderTimeout = 5 * time.Second
	defaultGinReadTimeout       = 15 * time.Second
	defaultGinIdleTimeout       = 60 * time.Second
	defaultGinMaxHeaderBytes    = 64 * 1024
	defaultGinMaxBodyBytes      = 1024 * 1024
	defaultGinMaxInFlight       = 256
)

// Gin defines HTTP server settings used to construct the local Gin adapter.
// The struct is intended to be loaded once and reused as immutable startup
// configuration.
type Gin struct {
	// Host is the HTTP listen hostname.
	Host string `yaml:"host"`

	// Port is the HTTP listen port.
	Port int `yaml:"port"`

	// TrustedProxies contains trusted reverse-proxy CIDRs or addresses.
	TrustedProxies []string `yaml:"trusted_proxies"`

	// Logger controls whether structured HTTP access logging is enabled.
	Logger bool `yaml:"logger"`

	// Mode configures the Gin runtime mode (debug, release, or test).
	Mode string `yaml:"mode"`

	// ReadHeaderTimeout configures the HTTP server read-header timeout.
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`

	// ReadTimeout bounds reading the complete request, including the body.
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout bounds writing the response. Zero leaves net/http's write
	// timeout disabled, which is useful for endpoints that may stream later.
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// IdleTimeout bounds keep-alive connections waiting for the next request.
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// MaxHeaderBytes bounds request-header memory consumed by net/http.
	MaxHeaderBytes int `yaml:"max_header_bytes"`

	// MaxBodyBytes is the hard REST request-body limit enforced before binding.
	MaxBodyBytes int64 `yaml:"max_body_bytes"`

	// MaxInFlight bounds requests executing concurrently inside this process.
	// The limiter is fail-fast and does not create an application-side queue.
	MaxInFlight int `yaml:"max_in_flight"`

	// Swagger configures the Swagger UI HTTP resource.
	Swagger Swagger `yaml:"swagger"`

	// I18N configures REST response localization.
	I18N I18N `yaml:"i18n"`

	// HTTPDebugLogger configures temporary Development-only diagnostic logging
	// of the full HTTP exchange without redaction. Keep this disabled outside
	// controlled debugging sessions with test data.
	//
	// Compile-time + runtime controls both apply: Production builds exclude the
	// middleware implementation and reject Enabled=true during validate.
	// Neither GIN_MODE nor APP_ENVIRONMENT may imply enablement.
	HTTPDebugLogger HTTPDebugLogger `yaml:"http_debug_logger"`

	// ResponseLogging configures optional allowlisted response-body enrichment
	// of AccessLogger records. Available in Production; disabled by default.
	//
	// This is not a second logging pipeline: when Enabled, AccessLogger attaches
	// http.response_body to the existing "http request completed" record.
	// Requires Logger=true (enforced in Gin.validate).
	ResponseLogging ResponseLogging `yaml:"response_logging"`
}

// I18N configures request language negotiation and the default response locale.
type I18N struct {
	// DefaultLanguage is used when neither Accept-Language nor lang selects a
	// supported locale. Empty means en-US for backwards compatibility.
	DefaultLanguage string `yaml:"default_language"`
}

func (i I18N) defaultLocale() (resti18n.Locale, error) {
	const op = "config.I18N.defaultLocale"

	if i.DefaultLanguage == "" {
		return resti18n.DefaultLocale, nil
	}

	locale, err := resti18n.ParseLocale(i.DefaultLanguage)
	if err != nil {
		return "", fmt.Errorf("%s: invalid gin.i18n.default_language %q: %w", op, i.DefaultLanguage, err)
	}

	return locale, nil
}

// Swagger configures exposure of the Swaggo-generated Swagger 2.0 UI.
type Swagger struct {
	// Enabled controls whether /swagger/*any is registered.
	//
	// Keep this false outside local and development environments.
	Enabled bool `yaml:"enabled"`
}

// HTTPDebugLogger configures Development-only diagnostic logging of the full
// HTTP exchange (request and response) without redaction.
//
// WARNING: When enabled, logs may contain credentials, OTPs, customer data,
// payment references, and other secrets. Enable only temporarily in controlled
// development environments using test data. Do not forward these logs to shared
// or centralized collectors without appropriate controls. The development build
// profile does not make captured data safe by itself.
//
// Production builds reject Enabled=true at validation time and do not compile
// the unredacted middleware implementation.
//
// Capture limits (Max*BodyBytes) bound memory retained for log attributes only.
// They never truncate the HTTP bytes delivered to handlers or clients.
type HTTPDebugLogger struct {
	// Enabled controls whether the Development HTTPDebugLogger middleware is
	// registered. Do not enable based on GIN_MODE or environment name.
	Enabled bool `yaml:"enabled"`

	// MaxRequestBodyBytes limits request-body bytes retained for diagnostic
	// logs. Larger bodies are truncated in the log representation only.
	MaxRequestBodyBytes int64 `yaml:"max_request_body_bytes"`

	// MaxResponseBodyBytes limits response-body bytes retained for diagnostic
	// logs. Client-visible responses are never truncated.
	MaxResponseBodyBytes int64 `yaml:"max_response_body_bytes"`
}

// ResponseLogging configures optional safe response-body enrichment of
// AccessLogger records via a strict allowlist. Disabled by default. When
// enabled, gin.logger must also be true.
//
// Unlike HTTPDebugLogger, this capability is compiled into Production builds.
// Safety comes from fail-closed allowlists and omission metadata, not from
// excluding the code. Defaults remain off so Production traffic is unchanged
// until an operator explicitly opts in.
type ResponseLogging struct {
	// Enabled controls whether AccessLogger captures and projects response bodies.
	Enabled bool `yaml:"enabled"`

	// MaxBodyBytes limits response bytes retained for allowlist projection.
	// Client-visible response size is unaffected.
	MaxBodyBytes int64 `yaml:"max_body_bytes"`
}

// NewEngine constructs a Gin HTTP engine configured with the selected mode.
// Panic recovery is always installed as the outermost middleware; optional
// logging remains configuration-driven. The returned engine is intended
// for setup during process startup before it begins serving requests.
//
// Registration order (outer → inner) is load-bearing for logging correctness:
//
//  1. PanicRecovery — outermost recover boundary; panics never reach net/http.
//  2. TraceID — correlation IDs for subsequent middleware and handlers.
//  3. Language — REST locale negotiation.
//  4. HTTPDebugLogger — Development-only; compile-time gated via appendHTTPDebugLogger.
//  5. AccessLogger — optional; may wrap the writer for allowlisted response projection.
//  6. AdmissionControl — inside logging so rejections remain observable, outside
//     ErrorHandler/routes so saturated processes do no handler work.
//  7. ErrorHandler — serializes public errors while the stack unwinds so
//     AccessLogger/HTTPDebugLogger observe final status and body bytes.
//
// Because Gin runs middleware as nested c.Next() calls, ErrorHandler executes
// inside AccessLogger's c.Next(). On unwind, ErrorHandler writes first, then
// AccessLogger records the completed exchange. PanicRecovery sits outside that
// nesting: a panic skips AccessLogger/HTTPDebugLogger post-processing.
func (g Gin) newEngine(
	provider trace.Provider,
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (*gin.Engine, error) {
	const op = "config.Gin.newEngine"

	defaultLocale, err := g.I18N.defaultLocale()
	if err != nil {
		return nil, err
	}

	localizer, err := i18nbuiltin.New(defaultLocale)
	if err != nil {
		return nil, fmt.Errorf("%s: construct REST localizer: %w", op, err)
	}

	// PanicRecovery is intentionally first so it protects every later middleware
	// and route handler. TraceID then enriches normal requests; if TraceID itself
	// panics, the recovery boundary still reports the failure without correlation
	// identifiers rather than letting the panic escape to net/http.
	middlewares := []gin.HandlerFunc{
		gingonic.CustomRecovery(middleware.PanicRecovery(logger, reporter)),
		middleware.TraceID(provider),
		middleware.Language(localizer, defaultLocale),
	}

	// Development builds may add the unredacted HTTP debug logger here. The
	// production implementation is compiled as a no-op, so changing runtime
	// configuration cannot enable this diagnostic capability in production.
	middlewares = g.appendHTTPDebugLogger(middlewares, logger)

	if g.Logger {
		middlewares = append(middlewares, middleware.AccessLogger(
			logger,
			middleware.AccessLoggerOptions{
				ResponseLogging: middleware.ResponseLoggingOptions{
					Enabled:      g.ResponseLogging.Enabled,
					MaxBodyBytes: g.ResponseLogging.MaxBodyBytes,
				},
			},
		))
	}

	// Admission control is registered inside request logging so rejected
	// requests remain observable, but outside ErrorHandler/routes so no
	// downstream work starts when the process is saturated.
	middlewares = append(
		middlewares,
		middleware.AdmissionControl(middleware.AdmissionControlOptions{
			MaxBodyBytes: g.getMaxBodyBytes(),
			MaxInFlight:  g.getMaxInFlight(),
		}),
	)

	// Register ErrorHandler after AccessLogger so it runs inside the access-log
	// middleware. On unwind, ErrorHandler writes the final error response first,
	// allowing AccessLogger to record the final status code and response size.
	middlewares = append(middlewares, middleware.ErrorHandler(logger, reporter))

	return gin.New(g.Mode, g.TrustedProxies, middlewares...)
}

// GetListeningAddress formats the configured host and port into the address
// string expected by Gin's Run method.
func (g Gin) getListeningAddress() string {
	return fmt.Sprintf("%s:%d", g.Host, g.Port)
}

// getReadHeaderTimeout returns the configured HTTP read-header timeout or the
// default used to mitigate Slowloris-style attacks.
func (g Gin) getReadHeaderTimeout() time.Duration {
	if g.ReadHeaderTimeout <= 0 {
		return defaultGinReadHeaderTimeout
	}

	return g.ReadHeaderTimeout
}

// getReadTimeout returns the configured full-request read timeout.
func (g Gin) getReadTimeout() time.Duration {
	if g.ReadTimeout <= 0 {
		return defaultGinReadTimeout
	}

	return g.ReadTimeout
}

// getWriteTimeout returns the configured response write timeout. Zero keeps the
// net/http default (no write deadline) so future streaming endpoints are not
// accidentally broken by a hidden default.
func (g Gin) getWriteTimeout() time.Duration {
	if g.WriteTimeout < 0 {
		return 0
	}

	return g.WriteTimeout
}

// getIdleTimeout returns the configured keep-alive idle timeout.
func (g Gin) getIdleTimeout() time.Duration {
	if g.IdleTimeout <= 0 {
		return defaultGinIdleTimeout
	}

	return g.IdleTimeout
}

// getMaxHeaderBytes returns the net/http header-size safety bound.
func (g Gin) getMaxHeaderBytes() int {
	if g.MaxHeaderBytes <= 0 {
		return defaultGinMaxHeaderBytes
	}

	return g.MaxHeaderBytes
}

// getMaxBodyBytes returns the hard maximum request-body size.
func (g Gin) getMaxBodyBytes() int64 {
	if g.MaxBodyBytes <= 0 {
		return defaultGinMaxBodyBytes
	}

	return g.MaxBodyBytes
}

// getMaxInFlight returns the process-local request concurrency bound.
func (g Gin) getMaxInFlight() int {
	if g.MaxInFlight <= 0 {
		return defaultGinMaxInFlight
	}

	return g.MaxInFlight
}
