package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	i18nbuiltin "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n/builtin"
)

func TestLanguage_ResolvesAndPropagatesLocale(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(middleware.Language(localizer, resti18n.LocaleENUS))
	engine.GET("/", func(c *gin.Context) {
		state, ok := resti18n.RequestStateFromContext(c.Request.Context())
		require.True(t, ok)
		assert.Equal(t, resti18n.LocaleFAIR, state.Locale)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(header.AcceptLanguage, "fa-IR")
	req.Header.Set(header.Language, "en-US")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "fa-IR", rec.Header().Get(header.ContentLanguage))
	assert.Contains(t, rec.Header().Values(header.Vary), header.AcceptLanguage)
	assert.Contains(t, rec.Header().Values(header.Vary), header.Language)
}
