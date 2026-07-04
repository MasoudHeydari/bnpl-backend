//go:build development

package swaggerrs

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegister_EnabledRegistersSwaggerRoute(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()

	Register(engine, RegisterOptions{Enabled: true})

	requireRouteRegistered(t, engine.Routes(), "GET", "/swagger/*any")
}

func TestRegister_DisabledRegistersNoSwaggerRoute(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()

	Register(engine, RegisterOptions{Enabled: false})

	requireRouteNotRegistered(t, engine.Routes(), "GET", "/swagger/*any")
}

func requireRouteRegistered(
	t *testing.T,
	routes []gin.RouteInfo,
	method, path string,
) {
	t.Helper()

	for _, route := range routes {
		if route.Method == method && route.Path == path {
			return
		}
	}

	t.Fatalf("route %s %s was not registered", method, path)
}

func requireRouteNotRegistered(
	t *testing.T,
	routes []gin.RouteInfo,
	method, path string,
) {
	t.Helper()

	for _, route := range routes {
		if route.Method == method && route.Path == path {
			t.Fatalf("route %s %s should not be registered", method, path)
		}
	}
}
