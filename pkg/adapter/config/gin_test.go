package config

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	going "github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	adaptertrace "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

func TestGin_getListeningAddress(t *testing.T) {
	t.Parallel()

	g := Gin{Host: "127.0.0.1", Port: 8080}
	assert.Equal(t, "127.0.0.1:8080", g.getListeningAddress())
}

func TestGin_getReadHeaderTimeout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, defaultGinReadHeaderTimeout, (Gin{}).getReadHeaderTimeout())

	custom := Gin{ReadHeaderTimeout: 10 * time.Second}
	assert.Equal(t, 10*time.Second, custom.getReadHeaderTimeout())
}

func TestGin_newEngine_MiddlewareCount(t *testing.T) {
	t.Parallel()

	provider := adaptertrace.NewULIDProvider()
	logger := lognoop.New()

	tests := []struct {
		name     string
		gin      Gin
		expected int
	}{
		{
			name:     "trace only",
			gin:      Gin{Mode: "test"},
			expected: 5, // PanicRecovery + TraceID + Language + AdmissionControl + ErrorHandler
		},
		{
			name:     "trace and logger",
			gin:      Gin{Mode: "test", Logger: true},
			expected: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			engine, err := tt.gin.newEngine(provider, logger, nil)
			require.NoError(t, err)
			require.NotNil(t, engine)
			assert.Len(t, engine.Handlers, tt.expected)
		})
	}
}

func TestI18N_defaultLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		expected string
		wantErr  bool
	}{
		{name: "empty uses default", expected: "en-US"},
		{name: "english US", value: "en-US", expected: "en-US"},
		{name: "english US case insensitive", value: "EN-us", expected: "en-US"},
		{name: "persian Iran", value: "fa-IR", expected: "fa-IR"},
		{name: "persian Iran case insensitive", value: "FA-ir", expected: "fa-IR"},
		{name: "generic english unsupported", value: "en", wantErr: true},
		{name: "generic persian unsupported", value: "fa", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			locale, err := (I18N{DefaultLanguage: tt.value}).defaultLocale()
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, locale.String())
		})
	}
}

func TestGin_newEngine_InvalidDefaultLanguage(t *testing.T) {
	t.Parallel()

	_, err := (Gin{
		Mode: "test",
		I18N: I18N{DefaultLanguage: "de"},
	}).newEngine(adaptertrace.NewULIDProvider(), lognoop.New(), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gin.i18n.default_language")
}

func TestGin_newEngine_PanicRecoveryPreservesTraceIDs(t *testing.T) {
	t.Parallel()

	g := Gin{Mode: "test"}
	engine, err := g.newEngine(adaptertrace.NewULIDProvider(), lognoop.New(), nil)
	require.NoError(t, err)

	engine.GET("/panic", func(c *going.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotEmpty(t, rec.Header().Get(header.TraceID))
	assert.NotEmpty(t, rec.Header().Get(header.RequestID))
	assert.NotEmpty(t, rec.Header().Get(header.CorrelationID))
}

func TestGin_newEngine_InvalidTrustedProxyReturnsError(t *testing.T) {
	t.Parallel()

	_, err := (Gin{
		Mode:           "test",
		TrustedProxies: []string{"not-a-cidr"},
	}).newEngine(adaptertrace.NewULIDProvider(), lognoop.New(), nil)

	require.Error(t, err)
}

func TestGin_validate_HTTPDebugLoggerRejectedInProductionEnvironment(t *testing.T) {
	t.Parallel()

	err := (Gin{
		HTTPDebugLogger: HTTPDebugLogger{Enabled: true},
	}).validate("production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gin.http_debug_logger.enabled must be false in production")
}

func TestGin_validate_ResponseLoggingRequiresAccessLogger(t *testing.T) {
	t.Parallel()

	err := (Gin{
		Logger:          false,
		ResponseLogging: ResponseLogging{Enabled: true},
	}).validate("development")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gin.response_logging.enabled requires gin.logger=true")

	require.NoError(t, (Gin{
		Logger:          true,
		ResponseLogging: ResponseLogging{Enabled: true},
	}).validate("development"))
}

type configRecordingLogger struct {
	mu      *sync.Mutex
	records *[]configLogRecord
	attrs   []any
}

type configLogRecord struct {
	msg   string
	attrs []any
}

func newConfigRecordingLogger() *configRecordingLogger {
	return &configRecordingLogger{
		mu:      &sync.Mutex{},
		records: &[]configLogRecord{},
	}
}

func (l *configRecordingLogger) With(args ...any) corelog.Logger {
	attrs := append(append([]any{}, l.attrs...), args...)
	return &configRecordingLogger{mu: l.mu, records: l.records, attrs: attrs}
}

func (l *configRecordingLogger) Debug(msg string, args ...any) { l.append(msg, args...) }
func (l *configRecordingLogger) Info(msg string, args ...any)  { l.append(msg, args...) }
func (l *configRecordingLogger) Warn(msg string, args ...any)  { l.append(msg, args...) }
func (l *configRecordingLogger) Error(msg string, args ...any) { l.append(msg, args...) }

func (l *configRecordingLogger) append(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attrs := append(append([]any{}, l.attrs...), args...)
	*l.records = append(*l.records, configLogRecord{msg: msg, attrs: attrs})
}

func (l *configRecordingLogger) snapshot() []configLogRecord {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]configLogRecord, len(*l.records))
	copy(out, *l.records)
	return out
}
