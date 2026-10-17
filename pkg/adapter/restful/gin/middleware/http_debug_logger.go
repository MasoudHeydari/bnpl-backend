//go:build development

package middleware

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

const (
	// defaultHTTPDebugBodyLimit bounds Development diagnostic retention for
	// both request and response captures when options leave limits unset.
	// "No redaction" does not mean unbounded memory: a misbehaving client or
	// large download must not pin megabytes into log attributes per request.
	defaultHTTPDebugBodyLimit = 64 * 1024

	// truncatedMarker is appended to truncated UTF-8 text captures so a human
	// reading the log cannot mistake a prefix for the complete payload.
	truncatedMarker = "...[TRUNCATED]"
)

// HTTPDebugLoggerOptions configures Development-only diagnostic logging of the
// full HTTP exchange.
//
// WARNING: HTTPDebugLogger records potentially sensitive request and response
// data without redaction. Enable it only for temporary debugging in controlled
// development environments using test data. Do not use it with production
// credentials, real customer information, or centralized/shared log
// destinations. Using the development build profile does not automatically make
// captured data safe — the operator is responsible for isolated environments
// and appropriate test credentials.
type HTTPDebugLoggerOptions struct {
	// MaxRequestBodyBytes caps how many request-body bytes are retained for
	// logging. Non-positive values fall back to defaultHTTPDebugBodyLimit.
	// Downstream handlers still receive the full body; only the log capture is
	// capped.
	MaxRequestBodyBytes int64

	// MaxResponseBodyBytes caps how many response-body bytes are retained for
	// logging. Non-positive values fall back to defaultHTTPDebugBodyLimit.
	// Client-visible response bytes are never truncated.
	MaxResponseBodyBytes int64
}

// HTTPDebugLogger logs the full HTTP exchange after the handler chain returns.
//
// Compile-time boundary:
// This middleware is compiled only into Development builds (//go:build
// development). Production binaries do not contain this implementation — see
// config.appendHTTPDebugLogger under the production build tag, which is a
// no-op and does not import this file. Runtime configuration alone must never
// be trusted to keep unredacted exchange logging out of Production; exclusion
// from the binary is the hard guarantee.
//
// Why Development-only and explicitly opt-in:
// Debugging BNPL HTTP flows often requires seeing Authorization headers,
// gateway identity headers, OTPs, checkout tokens in paths/referers, cookies,
// and exact JSON field values. Those values are unsafe for shared Production
// log pipelines. The feature therefore stays disabled by default and is never
// implied by GIN_MODE=debug or APP_ENVIRONMENT=development — an operator must
// set gin.http_debug_logger.enabled=true deliberately.
//
// What "full exchange" means here:
// Eligible request/response metadata and bodies are recorded without intentional
// redaction or sanitization, subject to (a) explicit capture size limits and
// (b) HTTP transport constraints (e.g. hijacked connections, unread request
// bodies). SanitizeHTTPPathForLog / SanitizeHTTPRefererForLog are deliberately
// not applied to the fields this middleware emits; those helpers remain for
// operational / error-reporting paths that still need them.
//
// Capture behavior:
//   - mirrors request body bytes the handler actually reads (bounded)
//   - mirrors response body bytes successfully written to the client (bounded)
//   - never alters client-visible request or response bytes
//   - reports truncation explicitly with limit and captured size metadata
//
// Request body mirroring uses capturingReadCloser: handlers keep a normal
// io.ReadCloser; only bytes they Read are copied into a capped buffer. The
// middleware never pre-drains or TeeReader-copies the full body before
// c.Next(), so streaming / large uploads are not forced into memory twice up
// front. Unread trailing body bytes are simply absent from the log (they were
// not observed by the application either).
//
// Response body mirroring uses capturingResponseWriter (shared with AccessLogger
// response logging). Write / WriteString paths are covered; client bytes are
// forwarded first, then a bounded prefix is retained for the log.
//
// Snapshot timing:
//   - Headers, query, path, and referer are cloned before c.Next() so later
//     middleware/handlers that mutate the request cannot erase diagnostic
//     evidence of what arrived.
//   - Auth identity fields are read after c.Next() so AuthContext / Attach
//     middleware deeper in the stack have already populated permission.Context.
//
// Security behavior (intentional Development product decision):
//   - no redaction or sanitization of headers, query strings, paths, cookies,
//     Authorization values, OTPs, tokens, JSON field values, or response bodies
//   - Do not enable against production credentials or real customer data
//
// Message text is "http request debug exchange" so AccessLogger can continue to
// emit its operational "http request completed" record without collapsing both
// into an ambiguous duplicate.
//
// Panic limitation: PanicRecovery is outermost. On panic, this middleware's
// post-c.Next() logging does not run, so a recovery-generated 500 body is not
// claimed as a captured debug exchange. PanicRecovery emits its own error log.
func HTTPDebugLogger(
	logger corelog.Logger,
	opts HTTPDebugLoggerOptions,
) gin.HandlerFunc {
	requestLimit := opts.MaxRequestBodyBytes
	if requestLimit <= 0 {
		requestLimit = defaultHTTPDebugBodyLimit
	}

	responseLimit := opts.MaxResponseBodyBytes
	if responseLimit <= 0 {
		responseLimit = defaultHTTPDebugBodyLimit
	}

	return func(c *gin.Context) {
		startedAt := time.Now()

		request := c.Request
		capturedRequest := newCapturingReadCloser(request.Body, requestLimit)
		if capturedRequest != nil {
			request.Body = capturedRequest
		}

		// Snapshot headers/query before handlers mutate the request. Auth
		// identity fields are read after c.Next() so Attach middleware has
		// already run.
		requestHeaders := cloneHeaders(request.Header)
		requestQuery := request.URL.Query().Encode()
		requestPath := request.URL.Path
		requestReferer := request.Referer()

		responseCapture := wrapResponseWriter(c.Writer, responseLimit)
		c.Writer = responseCapture

		c.Next()

		var requestBodyBytes []byte
		requestTruncated := false
		if capturedRequest != nil {
			requestBodyBytes = capturedRequest.Bytes()
			requestTruncated = capturedRequest.Truncated()
		}

		requestBody := formatDebugBody(
			request.Header.Get("Content-Type"),
			requestBodyBytes,
			requestTruncated,
			requestLimit,
		)

		responseBodyBytes := responseCapture.Bytes()
		responseBody := formatDebugBody(
			c.Writer.Header().Get("Content-Type"),
			responseBodyBytes,
			responseCapture.Truncated(),
			responseLimit,
		)
		// Hijack (or equivalent skip) means we must not present Bytes() as a
		// normal HTTP response body representation. Post-hijack net.Conn writes
		// are invisible here: zero captured bytes report body_presence=unknown,
		// never body_present=false.
		if reason := responseCapture.SkipReason(); reason != "" {
			skipped := map[string]any{
				"logged":        false,
				"reason":        reason,
				"captured_size": responseCapture.CapturedSize(),
				"limit_bytes":   responseLimit,
				"truncated":     responseCapture.Truncated(),
			}

			switch resolveBodyPresence(reason, responseCapture.CapturedSize(), responseCapture.Truncated()) {
			case bodyPresencePresent:
				skipped["body_present"] = true

			case bodyPresenceUnknown:
				skipped["body_presence"] = bodyPresenceUnknownValue

			default:
				skipped["body_present"] = false
			}

			responseBody = skipped
		}

		authFields := authLogFields(request.Context())

		entry := logger.With(
			"trace_id", trace.ID(request.Context()),
			"request_id", trace.RequestID(request.Context()),
			"correlation_id", trace.CorrelationID(request.Context()),

			"auth_subject_id", authFields.subjectID,
			"auth_tenant_id", authFields.tenantID,
			"auth_context_id", authFields.contextID,
			"auth_permissions_count", authFields.permissionsCount,

			"http.method", request.Method,
			"http.path", requestPath,
			"http.route", c.FullPath(),
			"http.query", requestQuery,
			"http.host", request.Host,
			"http.protocol", request.Proto,
			"http.scheme", requestScheme(request),
			"http.remote_address", request.RemoteAddr,
			"http.client_ip", c.ClientIP(),
			"http.user_agent", request.UserAgent(),
			"http.referer", requestReferer,
			"http.content_type", request.Header.Get("Content-Type"),
			"http.content_length", request.ContentLength,
			"http.request_headers", requestHeaders,
			"http.request_body", requestBody,
			"http.request_body_truncated", requestTruncated,
			"http.request_body_limit_bytes", requestLimit,
			"http.request_body_captured_size", len(requestBodyBytes),

			"http.status_code", c.Writer.Status(),
			"http.response_size", c.Writer.Size(),
			"http.response_headers", cloneHeaders(c.Writer.Header()),
			"http.response_body", responseBody,
			"http.response_body_truncated", responseCapture.Truncated(),
			"http.response_body_limit_bytes", responseLimit,
			"http.response_body_captured_size", responseCapture.CapturedSize(),
			"http.latency", time.Since(startedAt).String(),
			"http.errors", ginErrors(c.Errors),
		)

		// Release retained response capture bytes after attributes are built.
		responseCapture.ResetCapture()

		// Level by final status so 2xx stays Debug (opt-in noise), while client
		// and server failures surface at Warn/Error even when this logger is on.
		switch status := c.Writer.Status(); {
		case status >= http.StatusInternalServerError:
			entry.Error("http request debug exchange")

		case status >= http.StatusBadRequest:
			entry.Warn("http request debug exchange")

		default:
			entry.Debug("http request debug exchange")
		}
	}
}

// capturingReadCloser mirrors up to limit bytes of what the handler reads into
// an in-memory buffer used only for logging after the request finishes.
//
// It intentionally does not implement TeeReader-style full duplication before
// the handler runs: only observed Read bytes are retained. Returning the
// underlying Read's (n, err) unchanged keeps handler body parsing behavior
// identical to an unwrapped body.
type capturingReadCloser struct {
	body      io.ReadCloser
	captured  bytes.Buffer
	limit     int64
	truncated bool
}

func newCapturingReadCloser(body io.ReadCloser, limit int64) *capturingReadCloser {
	if body == nil {
		return nil
	}

	return &capturingReadCloser{body: body, limit: limit}
}

func (r *capturingReadCloser) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n <= 0 {
		return n, err
	}

	remaining := r.limit - int64(r.captured.Len())
	if remaining <= 0 {
		r.truncated = true
		return n, err
	}

	toCopy := int64(n)
	if toCopy > remaining {
		toCopy = remaining
		r.truncated = true
	}

	_, _ = r.captured.Write(p[:int(toCopy)])
	return n, err
}

func (r *capturingReadCloser) Close() error { return r.body.Close() }

func (r *capturingReadCloser) Bytes() []byte {
	return append([]byte(nil), r.captured.Bytes()...)
}

func (r *capturingReadCloser) Truncated() bool { return r.truncated }

// formatDebugBody chooses a log representation for a captured body prefix.
//
// Rules (Development, intentionally unredacted):
//   - Empty non-truncated capture → nil (no body observed).
//   - JSON: parse and emit the value tree with original field names/values.
//     If Unmarshal fails (often because truncation cut mid-token), emit Base64
//     of the retained prefix with reason invalid_or_truncated_json — never claim
//     parsed JSON is available.
//   - If JSON parses but truncated=true (prefix happened to be valid JSON while
//     more bytes were forwarded), mark complete=false so readers know the
//     capture is not the full wire body.
//   - form-urlencoded: parse query pairs without redacting keys/values.
//   - text/* with valid UTF-8: emit string; append truncatedMarker when capped.
//   - Everything else (binary / non-UTF-8): bounded Base64 with encoding metadata.
//     Arbitrary bytes are never reinterpreted as UTF-8 text.
func formatDebugBody(
	contentType string,
	body []byte,
	truncated bool,
	limit int64,
) any {
	if len(body) == 0 && !truncated {
		return nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}

	baseMeta := map[string]any{
		"content_type":  mediaType,
		"captured_size": len(body),
		"limit_bytes":   limit,
		"truncated":     truncated,
	}

	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		var value any
		if err := json.Unmarshal(body, &value); err != nil {
			baseMeta["logged"] = false
			baseMeta["reason"] = "invalid_or_truncated_json"
			baseMeta["encoding"] = "base64"
			baseMeta["value"] = base64.StdEncoding.EncodeToString(body)
			return baseMeta
		}

		if truncated {
			return map[string]any{
				"value":         value,
				"truncated":     true,
				"captured_size": len(body),
				"limit_bytes":   limit,
				"complete":      false,
			}
		}

		return value

	case mediaType == "application/x-www-form-urlencoded":
		values, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			baseMeta["logged"] = false
			baseMeta["reason"] = "unparseable_form_body"
			baseMeta["encoding"] = "base64"
			baseMeta["value"] = base64.StdEncoding.EncodeToString(body)
			return baseMeta
		}

		if truncated {
			return map[string]any{
				"value":         values,
				"truncated":     true,
				"captured_size": len(body),
				"limit_bytes":   limit,
				"complete":      false,
			}
		}

		return values

	case strings.HasPrefix(mediaType, "text/") && utf8.Valid(body):
		value := string(body)
		if truncated {
			value += truncatedMarker
			return map[string]any{
				"value":         value,
				"truncated":     true,
				"captured_size": len(body),
				"limit_bytes":   limit,
				"complete":      false,
			}
		}

		return value

	default:
		baseMeta["logged"] = true
		baseMeta["encoding"] = "base64"
		baseMeta["value"] = base64.StdEncoding.EncodeToString(body)
		return baseMeta
	}
}

// cloneHeaders deep-copies header values so later mutations of the live
// http.Header do not rewrite the snapshot retained for logging.
func cloneHeaders(headers http.Header) map[string]any {
	result := make(map[string]any, len(headers))

	for name, values := range headers {
		result[name] = append([]string(nil), values...)
	}

	return result
}

func requestScheme(request *http.Request) string {
	if request.TLS != nil {
		return "https"
	}

	if forwarded := request.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		return forwarded
	}

	return "http"
}

func ginErrors(errors []*gin.Error) []string {
	if len(errors) == 0 {
		return nil
	}

	result := make([]string, 0, len(errors))
	for _, err := range errors {
		result = append(result, err.Error())
	}

	return result
}

type authLogFieldValues struct {
	subjectID        string
	tenantID         string
	contextID        string
	permissionsCount int
}

// authLogFields reads gateway identity from the request context after auth
// middleware has run. Permissions are logged as a count only — the full
// permission set is high-cardinality and usually unnecessary for exchange
// debugging once subject/tenant/context are known.
func authLogFields(ctx context.Context) authLogFieldValues {
	authCtx, ok := permission.FromContext(ctx)
	if !ok {
		return authLogFieldValues{}
	}

	return authLogFieldValues{
		subjectID:        authCtx.SubjectID.String(),
		tenantID:         authCtx.TenantID.String(),
		contextID:        authCtx.ContextID.String(),
		permissionsCount: len(authCtx.Permissions),
	}
}
