package middleware

import (
	"bufio"
	"bytes"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

// defaultResponseBodyCaptureLimit is the fallback capture budget when a caller
// passes a non-positive MaxBodyBytes. It bounds memory retained for logging /
// allowlist projection only; it never truncates bytes delivered to the client.
const defaultResponseBodyCaptureLimit = 16 * 1024

// capturingResponseWriter is a bounded write-through wrapper around
// gin.ResponseWriter used by AccessLogger (Production-safe response logging)
// and by HTTPDebugLogger (Development diagnostics).
//
// Design goals:
//
//  1. Client-visible semantics must be byte-identical to an unwrapped writer.
//     Status, headers, and body bytes forwarded to the underlying writer are
//     never altered, buffered-before-send, or rewritten.
//  2. Capture is opportunistic and fail-closed for logging: if we cannot retain
//     a complete, trustworthy snapshot (limit exceeded, hijack, etc.), callers
//     must observe Truncated / SkipReason rather than treating Bytes() as a
//     full response.
//  3. Memory is bounded. Unlimited response buffering would turn logging into
//     an availability risk under large or streaming responses.
//
// Embedding gin.ResponseWriter promotes Status, Size, Written, WriteHeader,
// WriteHeaderNow, CloseNotify, and Pusher from the concrete Gin writer. Only
// Write, WriteString, Hijack, and Flush are overridden so capture participates
// in the paths Gin handlers typically use. The compile-time assertions at the
// bottom of this file document that Hijacker and Flusher remain satisfied.
//
// Capture timing: bytes are mirrored only after the underlying Write /
// WriteString reports how many bytes were successfully forwarded (n > 0).
// Failed or zero-length writes do not inflate the capture buffer. Partial
// success captures only data[:n] / s[:n], matching what the client likely
// received for that call.
//
// Nested wrappers: when HTTPDebugLogger and AccessLogger(response logging) are
// both enabled, AccessLogger wraps the already-wrapped debug writer. Each
// layer independently mirrors the same forwarded bytes into its own buffer.
// That duplication is intentional so each middleware can apply its own limit
// and post-processing without sharing mutable capture state.
//
// Hijacked connections disable further capture and record
// responseBodyReasonStreaming. After Hijack succeeds, the TCP connection is
// owned by the application protocol (e.g. WebSocket); continuing to treat
// later writes as an HTTP response body would be incorrect, so SkipReason
// forces callers to emit omission metadata instead of a false "complete body".
//
// Flush is forwarded unchanged. Flushing alone does not disable capture:
// subsequent Write/WriteString calls continue to be mirrored until the limit
// is reached or Hijack sets skipReason. Callers that stream indefinitely will
// eventually hit the capture limit and see Truncated() == true.
//
// Limitations (honest current behavior):
//   - io.ReaderFrom is not implemented on this wrapper. io.Copy therefore uses
//     the Write path (which is captured). If a future Gin or handler path
//     type-asserts past this wrapper to a ReaderFrom-capable concrete writer,
//     those bytes would bypass capture — that path is not used by the current
//     REST serializers.
//   - CloseNotify and Pusher are available only via promotion from the embedded
//     gin.ResponseWriter; this file does not re-implement them.
//   - Panic recovery is outermost in the Gin stack. If a panic unwinds before
//     post-c.Next() logging runs, captured buffers from this wrapper are never
//     observed by AccessLogger / HTTPDebugLogger (see those middlewares).
type capturingResponseWriter struct {
	gin.ResponseWriter

	// buf retains up to limit successfully forwarded body bytes for later
	// logging / projection. It is not the client response buffer.
	buf bytes.Buffer

	// limit is the maximum number of bytes retained in buf.
	limit int64

	// truncated is true once more body bytes were forwarded than buf retained.
	// When true, Bytes() is a prefix, never a complete response representation.
	truncated bool

	// skipReason, when non-empty, means capture is no longer trustworthy for
	// body projection (currently set after a successful Hijack). Callers must
	// prefer this over interpreting Bytes().
	skipReason string

	// totalWritten counts bytes successfully forwarded through Write /
	// WriteString since wrap time, including bytes discarded after the capture
	// limit. It is used to distinguish "no body was sent" from "body was sent
	// but not retained" when skipReason is set.
	totalWritten int64
}

// wrapResponseWriter installs a capturing wrapper. Non-positive limit falls
// back to defaultResponseBodyCaptureLimit so misconfiguration cannot disable
// the memory bound.
func wrapResponseWriter(w gin.ResponseWriter, limit int64) *capturingResponseWriter {
	if limit <= 0 {
		limit = defaultResponseBodyCaptureLimit
	}

	return &capturingResponseWriter{ResponseWriter: w, limit: limit}
}

// Write forwards data to the client, then mirrors only the successfully
// written prefix into the capture buffer. Returning the underlying (n, err)
// unchanged preserves Gin / net/http error semantics for the handler.
func (w *capturingResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.capture(data[:n])
	}

	return n, err
}

// WriteString mirrors Write for Gin's string path. Gin serializers and
// c.String use WriteString; capturing only Write would silently miss those
// bodies. The capture uses []byte(s)[:n] so a partial write does not claim
// more bytes than were forwarded.
func (w *capturingResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 {
		w.capture([]byte(s)[:n])
	}

	return n, err
}

// Hijack delegates to the underlying writer. On success, further body capture
// is disabled via skipReason because the connection is no longer a normal
// HTTP response stream owned by this middleware.
func (w *capturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, bufrw, err := w.ResponseWriter.Hijack()
	if err == nil {
		w.skipReason = responseBodyReasonStreaming
	}

	return conn, bufrw, err
}

// Flush forwards to Gin's Flusher implementation so chunked / streaming
// handlers that call Flush continue to work. Capture state is unchanged.
func (w *capturingResponseWriter) Flush() {
	w.ResponseWriter.Flush()
}

// capture appends chunk to buf until limit, then marks truncated. Bytes past
// the limit are still counted in totalWritten but not retained. When
// skipReason is set, capture is a no-op.
func (w *capturingResponseWriter) capture(chunk []byte) {
	if w.skipReason != "" {
		return
	}

	w.totalWritten += int64(len(chunk))

	remaining := w.limit - int64(w.buf.Len())
	if remaining <= 0 {
		w.truncated = true
		return
	}

	toCopy := int64(len(chunk))
	if toCopy > remaining {
		toCopy = remaining
		w.truncated = true
	}

	_, _ = w.buf.Write(chunk[:int(toCopy)])
}

// Bytes returns a copy of the retained capture prefix. Callers must check
// Truncated and SkipReason before treating this as a complete body.
func (w *capturingResponseWriter) Bytes() []byte {
	return append([]byte(nil), w.buf.Bytes()...)
}

// Truncated reports whether forwarded body bytes exceeded the capture limit.
func (w *capturingResponseWriter) Truncated() bool { return w.truncated }

// SkipReason returns a non-empty omission reason when capture was abandoned
// (for example after Hijack). Empty means capture rules still apply.
func (w *capturingResponseWriter) SkipReason() string { return w.skipReason }

// CapturedSize is the number of bytes currently retained in buf (≤ limit).
func (w *capturingResponseWriter) CapturedSize() int { return w.buf.Len() }

// TotalWritten is the cumulative count of successfully forwarded body bytes,
// including those not retained after the capture limit.
func (w *capturingResponseWriter) TotalWritten() int64 { return w.totalWritten }

// ResetCapture drops retained bytes from buf so large captures are not held
// longer than needed after projection / logging. It does not reset
// truncated, skipReason, or totalWritten; those describe what already
// happened on the wire for this request.
func (w *capturingResponseWriter) ResetCapture() {
	w.buf.Reset()
}

// Compile-time checks: the wrapper must keep satisfying the optional HTTP
// interfaces Gin relies on. If a future change drops Hijack or Flush, these
// assertions fail the build instead of silently breaking WebSocket / flush
// handlers at runtime.
var (
	_ http.Hijacker = (*capturingResponseWriter)(nil)
	_ http.Flusher  = (*capturingResponseWriter)(nil)
)
