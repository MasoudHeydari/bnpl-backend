package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// unmatchedRoute is the stable http.route value when Gin did not match a
// registered template. Using a constant avoids logging raw unmatched URLs
// (which may embed tokens) as high-cardinality route dimensions.
const unmatchedRoute = "<unmatched>"

// AccessLoggerOptions configures AccessLogger behavior beyond the default
// operational metadata fields.
type AccessLoggerOptions struct {
	// ResponseLogging optionally enriches the access record with a strictly
	// allowlisted, sanitized projection of the response body. Disabled by
	// default. When disabled, no response capture wrapper is installed.
	ResponseLogging ResponseLoggingOptions
}

// AccessLogger records one structured access log after a request completes.
//
// The middleware deliberately depends on the application's Logger abstraction
// instead of Gin's default writer-based logger. This keeps HTTP access logs on
// the same structured logging path as the rest of the service and avoids
// coupling the REST adapter to a concrete logging implementation.
//
// Access logs are always emitted at Info level. ErrorHandler owns error-level
// logging for unexpected server failures, so raising 5xx access logs to Error
// would duplicate the same failure in operational logs.
//
// Raw paths, query strings, headers, cookies, client IPs, user agents, and
// request bodies are intentionally excluded. HTTPDebugLogger (Development
// builds only) exists for explicitly enabled diagnostics that need the full
// unredacted HTTP exchange.
//
// When ResponseLogging is enabled, approved response fields are attached to
// this same access record under http.response_body. Unsupported, unknown, or
// oversized bodies are omitted with an explicit reason — never logged as raw
// JSON. Sensitive response headers (Set-Cookie, Location, Authorization) are
// never dumped.
//
// Independence from HTTPDebugLogger:
//   - AccessLogger always emits "http request completed" when registered.
//   - HTTPDebugLogger emits a separate "http request debug exchange" record.
//   - Production response logging enriches this access record; it does not
//     create a third logger or a second completion event.
//
// Middleware ordering (see config.Gin.newEngine):
//
//	PanicRecovery → TraceID → Language → HTTPDebugLogger? → AccessLogger? →
//	AdmissionControl → ErrorHandler → handlers
//
// AccessLogger calls c.Next() first so ErrorHandler (inner) can serialize the
// public error response while the stack unwinds. Only after c.Next() returns
// does AccessLogger read final status, size, and optional body projection.
// That is why AccessLogger can observe ErrorHandler's HTTP status: ErrorHandler
// runs inside AccessLogger's c.Next(), writes the response, then returns.
//
// Panic limitation: PanicRecovery is outermost. A panic unwinds past
// AccessLogger's post-c.Next() code, so this middleware does not emit an access
// record for panicked requests. PanicRecovery logs "http panic recovered"
// separately and AbortWithStatus(500). Do not assume the access log captured
// the recovery response body — it did not run.
//
// Nested writers in Development: when HTTPDebugLogger is also enabled it wraps
// c.Writer first; AccessLogger then wraps that wrapper if ResponseLogging is
// on. Each layer captures independently (see capturingResponseWriter).
func AccessLogger(logger corelog.Logger, opts AccessLoggerOptions) gin.HandlerFunc {
	responseLimit := opts.ResponseLogging.MaxBodyBytes
	if responseLimit <= 0 {
		responseLimit = defaultResponseBodyCaptureLimit
	}

	return func(c *gin.Context) {
		startedAt := time.Now()

		// Install capture only when response logging is explicitly enabled.
		// Skipping the wrapper preserves the historical AccessLogger hot path
		// (no per-request body buffer) when the feature is off.
		var capture *capturingResponseWriter
		if opts.ResponseLogging.Enabled {
			capture = wrapResponseWriter(c.Writer, responseLimit)
			c.Writer = capture
		}

		c.Next()

		if logger == nil || c.Request == nil {
			return
		}

		ctx := c.Request.Context()
		route := c.FullPath()
		if route == "" {
			route = unmatchedRoute
		}

		attrs := []any{
			"trace_id", trace.ID(ctx),
			"request_id", trace.RequestID(ctx),
			"correlation_id", trace.CorrelationID(ctx),
			"http.method", c.Request.Method,
			"http.route", route,
			"http.status_code", c.Writer.Status(),
			"http.request_size", c.Request.ContentLength,
			"http.response_size", c.Writer.Size(),
			"http.latency", time.Since(startedAt).String(),
			"http.aborted", c.IsAborted(),
		}

		if opts.ResponseLogging.Enabled {
			bodyLog := buildAccessResponseBodyLog(c, capture, responseLimit)
			attrs = append(attrs, "http.response_body", bodyLog.asAttrs())

			// Drop retained raw bytes promptly after projection so Production
			// access logs do not keep full buffers until GC of the request.
			if capture != nil {
				capture.ResetCapture()
			}
		}

		logger.Info("http request completed", attrs...)
	}
}

// buildAccessResponseBodyLog projects the captured response (if any) through
// the fail-closed allowlist pipeline. Policy matching uses c.FullPath() even
// when the logged http.route attribute was rewritten to unmatchedRoute, so
// unmatched requests never accidentally match a template policy.
func buildAccessResponseBodyLog(
	c *gin.Context,
	capture *capturingResponseWriter,
	limit int64,
) responseBodyLog {
	if capture == nil {
		return responseBodyLog{
			BodyPresent: false,
			Logged:      false,
			Reason:      responseBodyReasonCaptureDisabled,
		}
	}

	body := capture.Bytes()
	return projectResponseBody(
		c.Request.Method,
		c.FullPath(),
		c.Writer.Status(),
		c.Writer.Header(),
		body,
		capture.Truncated(),
		capture.SkipReason(),
		limit,
	)
}
