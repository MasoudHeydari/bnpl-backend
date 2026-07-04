package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectResponseBody_BodyPresenceStates(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set("Content-Type", "application/json")

	t.Run("ordinary empty body is absent", func(t *testing.T) {
		t.Parallel()

		got := projectResponseBody(
			http.MethodGet,
			"/healthz/live",
			http.StatusNoContent,
			header,
			nil,
			false,
			"",
			1024,
		)
		attrs := got.asAttrs()

		assert.Equal(t, false, attrs["body_present"])
		assert.Equal(t, false, attrs["logged"])
		assert.Equal(t, responseBodyReasonEmptyBody, attrs["reason"])
		_, hasPresence := attrs["body_presence"]
		assert.False(t, hasPresence)
	})

	t.Run("hijack with observed bytes is present", func(t *testing.T) {
		t.Parallel()

		got := projectResponseBody(
			http.MethodGet,
			"/ws",
			http.StatusSwitchingProtocols,
			header,
			[]byte(`upgrade-prefix`),
			false,
			responseBodyReasonStreaming,
			1024,
		)
		attrs := got.asAttrs()

		assert.Equal(t, true, attrs["body_present"])
		assert.Equal(t, false, attrs["logged"])
		assert.Equal(t, responseBodyReasonStreaming, attrs["reason"])
		_, hasPresence := attrs["body_presence"]
		assert.False(t, hasPresence)
	})

	t.Run("hijack without observed bytes is unknown", func(t *testing.T) {
		t.Parallel()

		got := projectResponseBody(
			http.MethodGet,
			"/ws",
			http.StatusSwitchingProtocols,
			header,
			nil,
			false,
			responseBodyReasonStreaming,
			1024,
		)
		attrs := got.asAttrs()

		assert.Equal(t, bodyPresenceUnknownValue, attrs["body_presence"])
		assert.Equal(t, false, attrs["logged"])
		assert.Equal(t, responseBodyReasonStreaming, attrs["reason"])
		_, hasPresent := attrs["body_present"]
		assert.False(t, hasPresent, "unknown must not emit body_present=false")
	})
}

func TestAccessLogger_HijackBodyPresenceUnknown(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/ws", func(c *gin.Context) {
		conn, _, err := c.Writer.Hijack()
		require.NoError(t, err)
		require.NotNil(t, conn)
		_ = conn.Close()
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	engine.ServeHTTP(&hijackableResponseRecorder{ResponseRecorder: httptest.NewRecorder()}, req)

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, bodyPresenceUnknownValue, body["body_presence"])
	assert.Equal(t, false, body["logged"])
	assert.Equal(t, responseBodyReasonStreaming, body["reason"])
	_, hasPresent := body["body_present"]
	assert.False(t, hasPresent)
}

func TestLookupResponseLogPolicy_ConcurrentReaders(t *testing.T) {
	t.Parallel()

	const goroutines = 32
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				_, ok := lookupResponseLogPolicy(http.MethodGet, "/healthz/live")
				if !ok {
					t.Errorf("expected healthz/live policy")
					return
				}

				_, ok = lookupResponseLogPolicy(http.MethodGet, "/not-a-policy-route")
				if ok {
					t.Errorf("unexpected policy match")
					return
				}
			}
		}()
	}

	wg.Wait()
}
