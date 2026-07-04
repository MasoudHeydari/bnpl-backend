package bootstrap

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterRoutes_HealthEnabled(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Usecases.Health.Enabled = true
	cfg.Gin.Swagger.Enabled = false

	app := testAppWithInfrastructure(t, cfg)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	engine, err := cfg.NewGinEngine(app.Infrastructure.Trace, app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)

	err = app.RegisterRoutes(engine)
	require.NoError(t, err)

	routes := engine.Routes()
	requireRouteRegistered(t, routes, "GET", "/healthz/live")
	requireRouteRegistered(t, routes, "GET", "/healthz/ready")
	requireRouteRegistered(t, routes, "POST", "/api/v1/partner/checkout-sessions")
	requireRouteRegistered(t, routes, "GET", "/api/v1/partner/checkout-sessions/:checkout_session_id")
	requireRouteRegistered(t, routes, "POST", "/api/v1/partner/checkout-sessions/:checkout_session_id/cancel")
	requireRouteRegistered(t, routes, "PUT", "/api/v1/partner/return-destinations/:destination_name")
	requireRouteRegistered(t, routes, "PUT", "/api/v1/partner/webhook-destination")
	requireRouteRegistered(t, routes, "GET", "/api/v1/checkout-sessions/:token")
	requireRouteRegistered(t, routes, "GET", "/api/v1/checkout-sessions/:token/return")
	requireRouteRegistered(t, routes, "POST", "/api/v1/checkout-sessions/:token/start")
	requireRouteRegistered(t, routes, "POST", "/api/v1/checkout-sessions/:token/confirm")
	requireRouteRegistered(t, routes, "POST", "/api/v1/checkout-sessions/:token/cancel")
	requireRouteNotRegistered(t, routes, "POST", "/api/v1/internal/checkout-sessions")
	requireRouteNotRegistered(t, routes, "GET", "/api/v1/hosted/checkout-sessions/:checkout_session_id")
	requireRouteRegistered(t, routes, "POST", "/api/v1/checkout-decisions")
	requireRouteNotRegistered(t, routes, "POST", "/api/v1/checkout-decisions/:decision_id/installments/offer")
	requireRouteRegistered(t, routes, "POST", "/api/v1/customer-credit/initiate")
	requireRouteRegistered(t, routes, "GET", "/api/v1/payments")
	requireRouteNotRegistered(t, routes, "GET", "/swagger/*any")
}

func TestRegisterRoutes_HealthDisabled(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Usecases.Health.Enabled = false
	cfg.Gin.Swagger.Enabled = false

	app := testAppWithInfrastructure(t, cfg)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	engine, err := cfg.NewGinEngine(app.Infrastructure.Trace, app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)

	err = app.RegisterRoutes(engine)
	require.NoError(t, err)

	routes := engine.Routes()
	requireRouteNotRegistered(t, routes, "GET", "/healthz/live")
	requireRouteNotRegistered(t, routes, "GET", "/healthz/ready")
}

func TestRegisterRoutes_SwaggerEnabled(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Gin.Swagger.Enabled = true

	app := testAppWithInfrastructure(t, cfg)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	engine, err := cfg.NewGinEngine(app.Infrastructure.Trace, app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)

	err = app.RegisterRoutes(engine)
	require.NoError(t, err)

	requireRouteRegistered(t, engine.Routes(), "GET", "/swagger/*any")
}

func TestRegisterRoutes_SwaggerDisabled(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Gin.Swagger.Enabled = false

	app := testAppWithInfrastructure(t, cfg)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	engine, err := cfg.NewGinEngine(app.Infrastructure.Trace, app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)

	err = app.RegisterRoutes(engine)
	require.NoError(t, err)

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
