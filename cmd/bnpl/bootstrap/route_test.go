package bootstrap

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterRoutes_HealthEnabled(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Usecases.Health.Enabled = true

	app := testAppWithInfrastructure(t, cfg)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	engine := cfg.NewGinEngine(app.Infrastructure.Trace)

	err = app.RegisterRoutes(context.Background(), engine)
	require.NoError(t, err)

	routes := engine.Routes()
	requireRouteRegistered(t, routes, "GET", "/healthz/live")
	requireRouteRegistered(t, routes, "GET", "/healthz/ready")
}

func TestRegisterRoutes_HealthDisabled(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Usecases.Health.Enabled = false

	app := testAppWithInfrastructure(t, cfg)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	engine := cfg.NewGinEngine(app.Infrastructure.Trace)

	err = app.RegisterRoutes(context.Background(), engine)
	require.NoError(t, err)

	routes := engine.Routes()
	requireRouteNotRegistered(t, routes, "GET", "/healthz/live")
	requireRouteNotRegistered(t, routes, "GET", "/healthz/ready")
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
