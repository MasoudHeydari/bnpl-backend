package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	i18nbuiltin "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n/builtin"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

func TestAuthContext_Attach_ValidHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	subjectID := model.SubjectID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())

	engine := newLocalizedEngine(t)
	authMW := middleware.AuthContext()
	engine.Use(authMW.Attach())
	engine.GET("/protected", func(c *gin.Context) {
		authCtx, ok := permission.FromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, subjectID, authCtx.SubjectID)
		require.Equal(t, tenantID, authCtx.TenantID)
		require.Equal(t, contextID, authCtx.ContextID)
		require.Equal(t, []string{
			permission.DecisionsCreateSelf.Code,
			permission.PaymentsStartSelf.Code,
		}, authCtx.Permissions)

		authCtx.Permissions[0] = "mutated"
		again, ok := permission.FromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, permission.DecisionsCreateSelf.Code, again.Permissions[0])

		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(header.AuthSubjectID, subjectID.String())
	req.Header.Set(header.AuthTenantID, tenantID.String())
	req.Header.Set(header.AuthContextID, contextID.String())
	req.Header.Set(
		header.AuthPermissions,
		" "+permission.DecisionsCreateSelf.Code+" , "+permission.PaymentsStartSelf.Code+" , "+permission.DecisionsCreateSelf.Code+" ",
	)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestAuthContext_Attach_HeaderFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	valid := func() map[string]string {
		return map[string]string{
			header.AuthSubjectID:   uuid.New().String(),
			header.AuthTenantID:    uuid.New().String(),
			header.AuthContextID:   uuid.New().String(),
			header.AuthPermissions: permission.CreditLimitsCreateSelf.Code,
		}
	}

	cases := []struct {
		name   string
		mutate func(map[string]string)
		field  string
		code   string
	}{
		{
			name: "missing subject",
			mutate: func(h map[string]string) {
				delete(h, header.AuthSubjectID)
			},
			field: header.AuthSubjectID,
			code:  "REQUIRED",
		},
		{
			name: "invalid subject",
			mutate: func(h map[string]string) {
				h[header.AuthSubjectID] = "not-a-uuid"
			},
			field: header.AuthSubjectID,
			code:  "INVALID",
		},
		{
			name: "missing tenant",
			mutate: func(h map[string]string) {
				delete(h, header.AuthTenantID)
			},
			field: header.AuthTenantID,
			code:  "REQUIRED",
		},
		{
			name: "invalid tenant",
			mutate: func(h map[string]string) {
				h[header.AuthTenantID] = "bad"
			},
			field: header.AuthTenantID,
			code:  "INVALID",
		},
		{
			name: "missing context",
			mutate: func(h map[string]string) {
				delete(h, header.AuthContextID)
			},
			field: header.AuthContextID,
			code:  "REQUIRED",
		},
		{
			name: "invalid context",
			mutate: func(h map[string]string) {
				h[header.AuthContextID] = "bad"
			},
			field: header.AuthContextID,
			code:  "INVALID",
		},
		{
			name: "nil subject uuid",
			mutate: func(h map[string]string) {
				h[header.AuthSubjectID] = uuid.Nil.String()
			},
			field: header.AuthSubjectID,
			code:  "INVALID",
		},
		{
			name: "nil tenant uuid",
			mutate: func(h map[string]string) {
				h[header.AuthTenantID] = uuid.Nil.String()
			},
			field: header.AuthTenantID,
			code:  "INVALID",
		},
		{
			name: "nil context uuid",
			mutate: func(h map[string]string) {
				h[header.AuthContextID] = uuid.Nil.String()
			},
			field: header.AuthContextID,
			code:  "INVALID",
		},
		{
			name: "missing permissions",
			mutate: func(h map[string]string) {
				delete(h, header.AuthPermissions)
			},
			field: header.AuthPermissions,
			code:  "REQUIRED",
		},
		{
			name: "empty permissions entries",
			mutate: func(h map[string]string) {
				h[header.AuthPermissions] = " , , "
			},
			field: header.AuthPermissions,
			code:  "REQUIRED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newLocalizedEngine(t)
			engine.Use(middleware.AuthContext().Attach())
			engine.GET("/protected", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			headers := valid()
			tc.mutate(headers)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			for name, value := range headers {
				req.Header.Set(name, value)
			}

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)

			var body serdser.ErrorResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.Equal(t, string(cerr.CodeUnauthorized), body.Code)
			require.Equal(t, "Authentication is required.", body.Message)
			require.NotNil(t, body.Details)
			require.Equal(t, []string{tc.field}, body.Details.Fields)
			require.Len(t, body.Details.Violations, 1)
			require.Equal(t, tc.code, body.Details.Violations[0].Code)
			require.Equal(t, []string{tc.field}, body.Details.Violations[0].Fields)
		})
	}
}

func TestAuthContext_RequireAny_BeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := newLocalizedEngine(t)
	authMW := middleware.AuthContext()
	engine.Use(authMW.Attach())

	handlerCalled := false
	engine.GET(
		"/checkout",
		authMW.RequireAny(permission.DecisionsCreateSelf),
		func(c *gin.Context) {
			handlerCalled = true
			c.Status(http.StatusNoContent)
		},
	)

	t.Run("allowed", func(t *testing.T) {
		handlerCalled = false
		req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
		setAuthHeaders(req, permission.DecisionsCreateSelf.Code)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.True(t, handlerCalled)
	})

	t.Run("forbidden", func(t *testing.T) {
		handlerCalled = false
		req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
		setAuthHeaders(req, permission.PaymentsStartSelf.Code)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusForbidden, recorder.Code)
		require.False(t, handlerCalled)
	})
}

func TestAuthContext_PublicRoutesUnaffected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := newLocalizedEngine(t)
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	v1 := engine.Group("/api/v1")
	authMW := middleware.AuthContext()
	v1.Use(authMW.Attach())
	v1.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz/live", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func setAuthHeaders(req *http.Request, permissions string) {
	req.Header.Set(header.AuthSubjectID, uuid.New().String())
	req.Header.Set(header.AuthTenantID, uuid.New().String())
	req.Header.Set(header.AuthContextID, uuid.New().String())
	req.Header.Set(header.AuthPermissions, permissions)
}

func newLocalizedEngine(t *testing.T) *gin.Engine {
	t.Helper()
	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(middleware.Language(localizer, resti18n.LocaleENUS))
	return engine
}
