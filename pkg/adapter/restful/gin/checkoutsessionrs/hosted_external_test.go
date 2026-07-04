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
		"GET /api/v1/checkout-sessions/:token/confirmation-status",
		"POST /api/v1/checkout-sessions/:token/otp",
		"POST /api/v1/checkout-sessions/:token/start",
		"POST /api/v1/checkout-sessions/:token/confirm",
		"POST /api/v1/checkout-sessions/:token/cancel",
	}

	for _, route := range want {
		require.True(t, hasRoute(routes, route), "missing route %s", route)
	}
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

func TestBindHostedExternalStartRequest_EmptyBody(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", http.NoBody)

	req, ok := bindHostedExternalStartRequest(c)
	require.True(t, ok)
	require.Equal(t, 0, req.EvaluationGeneration)
}

func TestBindHostedExternalStartRequest_EmptyObject(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	c.Request.Header.Set("Content-Type", "application/json")

	req, ok := bindHostedExternalStartRequest(c)
	require.True(t, ok)
	require.Equal(t, 0, req.EvaluationGeneration)
}

func TestBindHostedExternalStartRequest_WithGeneration(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"evaluation_generation":2}`))
	c.Request.Header.Set("Content-Type", "application/json")

	req, ok := bindHostedExternalStartRequest(c)
	require.True(t, ok)
	require.Equal(t, 2, req.EvaluationGeneration)
}

func TestBindHostedExternalStartRequest_InvalidJSON(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))
	c.Request.Header.Set("Content-Type", "application/json")

	_, ok := bindHostedExternalStartRequest(c)
	require.False(t, ok)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func hasRoute(routes gin.RoutesInfo, want string) bool {
	for _, route := range routes {
		if route.Method+" "+route.Path == want {
			return true
		}
	}

	return false
}
