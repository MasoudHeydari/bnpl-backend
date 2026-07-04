//go:build production

package config

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// appendHTTPDebugLogger is intentionally a no-op in Production builds.
//
// The unredacted HTTPDebugLogger middleware implementation lives in a
// Development-only source file and is not linked into Production binaries.
// Keeping this stub means Gin.newEngine can share one registration call site
// across build profiles without importing Development diagnostic code.
//
// Runtime defense in depth: Gin.validate also rejects
// gin.http_debug_logger.enabled=true when isProductionMode is true. Even if a
// misconfigured YAML sets Enabled, Production must neither register nor
// compile the raw exchange logger.
func (g Gin) appendHTTPDebugLogger(
	middlewares []gin.HandlerFunc,
	_ corelog.Logger,
) []gin.HandlerFunc {
	return middlewares
}
