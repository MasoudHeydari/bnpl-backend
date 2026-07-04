package gin

import "github.com/gin-gonic/gin"

type (
	// Engine aliases gin.Engine so the adapter package can expose a stable local
	// type name without reimplementing the server.
	Engine = gin.Engine
	// HandlerFunc aliases gin.HandlerFunc for local adapter cohesion.
	HandlerFunc = gin.HandlerFunc
)

// New constructs a Gin engine, sets mode, applies middleware, and configures
// trusted proxies. Invalid proxy configuration is returned as a startup error.
func New(
	mode string,
	trustedProxies []string,
	middlewares ...HandlerFunc,
) (*Engine, error) {
	e := gin.New()
	gin.SetMode(mode)
	if err := e.SetTrustedProxies(trustedProxies); err != nil {
		return nil, err
	}

	e.Use(middlewares...)
	return e, nil
}
