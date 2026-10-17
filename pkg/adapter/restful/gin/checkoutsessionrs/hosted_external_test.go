package checkoutsessionrs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

func TestRegisterCustomerToken_RoutesAreRegisteredWithoutAuthMiddleware(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterCustomerToken(engine.Group("/api/v1"), &checkoutsessionuc.UseCase{}, RegisterOptions{})

	routes := engine.Routes()
	want := []string{
		"GET /api/v1/checkout-sessions/:token",
		"GET /api/v1/checkout-sessions/:token/return",
		"POST /api/v1/checkout-sessions/:token/start",
		"POST /api/v1/checkout-sessions/:token/confirm",
		"POST /api/v1/checkout-sessions/:token/cancel",
	}

	for _, route := range want {
		require.True(t, hasRoute(routes, route), "missing route %s", route)
	}
}

func TestRegisterCustomerToken_ReturnRouteIsRegistered(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterCustomerToken(engine.Group("/api/v1"), &checkoutsessionuc.UseCase{}, RegisterOptions{})

	require.True(t, hasRoute(engine.Routes(), "GET /api/v1/checkout-sessions/:token/return"))
}

func TestRegisterCustomerToken_ConfirmRejectsMissingOTPBeforeUseCase(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterCustomerToken(engine.Group("/api/v1"), &checkoutsessionuc.UseCase{}, RegisterOptions{})

	body := `{"evaluation_generation":1}`
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/checkout-sessions/cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef/confirm",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func hasRoute(routes gin.RoutesInfo, want string) bool {
	for _, route := range routes {
		if route.Method+" "+route.Path == want {
			return true
		}
	}

	return false
}
