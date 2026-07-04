package middleware

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapturingResponseWriter_ResetCaptureReleasesBackingCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	capture := wrapResponseWriter(ctx.Writer, 4096)

	payload := bytes.Repeat([]byte("x"), 2048)
	n, err := capture.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.Equal(t, payload, recorder.Body.Bytes())

	require.Greater(t, capture.buf.Cap(), 0, "precondition: capture retained capacity")
	require.Equal(t, len(payload), capture.CapturedSize())
	require.Equal(t, int64(len(payload)), capture.TotalWritten())

	// Force metadata that must survive ResetCapture.
	capture.truncated = true
	capture.skipReason = responseBodyReasonStreaming
	totalWritten := capture.TotalWritten()

	capture.ResetCapture()

	assert.Equal(t, 0, capture.buf.Len())
	assert.Equal(t, 0, capture.buf.Cap(), "backing capacity must not be retained after ResetCapture")
	assert.Equal(t, 0, capture.CapturedSize())
	assert.Equal(t, totalWritten, capture.TotalWritten())
	assert.True(t, capture.Truncated())
	assert.Equal(t, responseBodyReasonStreaming, capture.SkipReason())

	// Client-visible bytes remain unchanged by ResetCapture.
	assert.Equal(t, payload, recorder.Body.Bytes())
}

func TestCapturingResponseWriter_HijackSetsStreamingSkipReason(t *testing.T) {
	gin.SetMode(gin.TestMode)

	hijackable := &hijackableResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	ctx, _ := gin.CreateTestContext(hijackable)
	capture := wrapResponseWriter(ctx.Writer, 1024)

	conn, _, err := capture.Hijack()
	require.NoError(t, err)
	require.NotNil(t, conn)
	t.Cleanup(func() { _ = conn.Close() })

	assert.Equal(t, responseBodyReasonStreaming, capture.SkipReason())
	assert.Equal(t, bodyPresenceUnknown, resolveBodyPresence(capture.SkipReason(), capture.CapturedSize(), capture.Truncated()))
}

func TestCapturingResponseWriter_ObservedBytesBeforeStreamingSkipArePresent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	capture := wrapResponseWriter(ctx.Writer, 1024)

	_, err := capture.Write([]byte(`{"ok":true}`))
	require.NoError(t, err)

	// Gin forbids Hijack after the response body has been written. Simulate
	// capture abandonment the same way Hijack does once skipReason is set.
	capture.skipReason = responseBodyReasonStreaming

	assert.Equal(t, bodyPresencePresent, resolveBodyPresence(
		capture.SkipReason(),
		capture.CapturedSize(),
		capture.Truncated(),
	))
}

// hijackableResponseRecorder satisfies http.Hijacker for capture tests.
type hijackableResponseRecorder struct {
	*httptest.ResponseRecorder
}

func (r *hijackableResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	server, client := net.Pipe()
	_ = client.Close()

	return server, bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server)), nil
}

var (
	_ http.ResponseWriter = (*hijackableResponseRecorder)(nil)
	_ http.Hijacker       = (*hijackableResponseRecorder)(nil)
)
