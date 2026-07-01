package gin

import (
	"log/slog"

	ginlogger "github.com/FabienMht/ginslog/logger"
	ginrecovery "github.com/FabienMht/ginslog/recovery"
	"github.com/gin-gonic/gin"
)

type (
	// Engine aliases gin.Engine so the adapter package can expose a stable local
	// type name without reimplementing the server.
	Engine = gin.Engine
	// HandlerFunc aliases gin.HandlerFunc for local adapter cohesion.
	HandlerFunc = gin.HandlerFunc
)

// New constructs a Gin engine, sets mode, applies middleware, and configures
// trusted proxies. Trusted proxies are currently hardcoded to 127.0.0.1;
// config.Gin does not yet expose trusted_proxies from YAML.
func New(
	mode string,
	trustedProxies []string,
	middlewares ...HandlerFunc,
) *Engine {
	e := gin.New()
	gin.SetMode(mode)
	_ = e.SetTrustedProxies(trustedProxies)
	e.Use(middlewares...)
	return e
}

// Logger middleware logs incoming requests and their responses
// which is useful for debugging.
func Logger() HandlerFunc {
	return ginlogger.New(slog.Default())
}

// Recovery middleware recovers panics and responds with 500 to clients.
func Recovery() HandlerFunc {
	return ginrecovery.New(slog.Default())
}
