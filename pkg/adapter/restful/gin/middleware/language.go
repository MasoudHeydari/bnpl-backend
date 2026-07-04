package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
)

// Language resolves the request locale and attaches localization state to the
// standard request context for downstream REST serializers.
//
// Accept-Language has precedence over the legacy lang header. Unsupported or
// missing values fall back to defaultLocale and never fail the business request.
func Language(localizer resti18n.Localizer, defaultLocale resti18n.Locale) gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := resti18n.ResolveLocale(
			c.GetHeader(header.AcceptLanguage),
			c.GetHeader(header.Language),
			defaultLocale,
		)

		ctx := resti18n.WithRequestState(c.Request.Context(), locale, localizer)
		c.Request = c.Request.WithContext(ctx)

		c.Header(header.ContentLanguage, locale.String())
		appendVary(c, header.AcceptLanguage)
		appendVary(c, header.Language)

		c.Next()
	}
}

func appendVary(c *gin.Context, value string) {
	for _, existing := range c.Writer.Header().Values(header.Vary) {
		if existing == value {
			return
		}
	}

	c.Writer.Header().Add(header.Vary, value)
}
