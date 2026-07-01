package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gogin "github.com/gin-gonic/gin"
	adaptertrace "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGin_getListeningAddress(t *testing.T) {
	t.Parallel()

	g := Gin{Host: "127.0.0.1", Port: 8080}
	assert.Equal(t, "127.0.0.1:8080", g.getListeningAddress())
}

func TestGin_newEngine_MiddlewareCount(t *testing.T) {
	t.Parallel()

	provider := adaptertrace.NewULIDProvider()

	tests := []struct {
		name     string
		gin      Gin
		expected int
	}{
		{
			name:     "trace only",
			gin:      Gin{Mode: "test"},
			expected: 1,
		},
		{
			name:     "trace and recovery",
			gin:      Gin{Mode: "test", Recovery: true},
			expected: 2,
		},
		{
			name:     "trace and logger",
			gin:      Gin{Mode: "test", Logger: true},
			expected: 2,
		},
		{
			name:     "trace recovery and logger",
			gin:      Gin{Mode: "test", Recovery: true, Logger: true},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			engine := tt.gin.newEngine(provider)
			require.NotNil(t, engine)
			assert.Len(t, engine.Handlers, tt.expected)
		})
	}
}

func TestGin_newEngine_TraceIDRunsBeforeRecovery(t *testing.T) {
	t.Parallel()

	g := Gin{Mode: "test", Recovery: true}
	engine := g.newEngine(adaptertrace.NewULIDProvider())

	engine.GET("/panic", func(c *gogin.Context) {
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
