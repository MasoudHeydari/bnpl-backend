package middleware

import (
	"github.com/gin-gonic/gin"
)

// HostedExternalHardening sets browser response headers for Hosted External
// routes addressed by a public checkout token in the URL.
//
// Cache-Control: no-store prevents intermediary/browser caching of checkout
// context. Referrer-Policy: no-referrer prevents the cs_ token from leaking
// through the Referer header on subsequent navigations.
func HostedExternalHardening() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}
