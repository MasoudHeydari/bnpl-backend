//go:build development

package config

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// appendHTTPDebugLogger registers the unredacted HTTPDebugLogger middleware
// when gin.http_debug_logger.enabled is true.
//
// This file is compiled only into Development builds. The matching Production
// file provides a no-op with the same method signature so Gin.newEngine can
// call appendHTTPDebugLogger unconditionally without importing Development-only
// middleware symbols into Production binaries.
//
// Enablement remains an explicit configuration decision even in Development:
// building with -tags=development does not register this middleware unless
// Enabled is true. That separation keeps local default runs quiet and forces
// operators to opt into sensitive exchange logging deliberately.
func (g Gin) appendHTTPDebugLogger(
	middlewares []gin.HandlerFunc,
	logger corelog.Logger,
) []gin.HandlerFunc {
	if !g.HTTPDebugLogger.Enabled {
		return middlewares
	}

	return append(
		middlewares,
		middleware.HTTPDebugLogger(
			logger,
			middleware.HTTPDebugLoggerOptions{
				MaxRequestBodyBytes:  g.HTTPDebugLogger.MaxRequestBodyBytes,
				MaxResponseBodyBytes: g.HTTPDebugLogger.MaxResponseBodyBytes,
			},
		),
	)
}
