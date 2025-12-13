package gin

import (
	"log/slog"

	ginlogger "github.com/FabienMht/ginslog/logger"
	ginrecovery "github.com/FabienMht/ginslog/recovery"
	"github.com/gin-gonic/gin"
)

type (
	Engine      = gin.Engine
	HandlerFunc = gin.HandlerFunc
)

func New(mode string, middlewares ...HandlerFunc) *Engine {
	e := gin.New()
	gin.SetMode(mode)
	_ = e.SetTrustedProxies([]string{"127.0.0.1"})
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
