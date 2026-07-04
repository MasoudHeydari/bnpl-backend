package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

const testTerminalAPIKey = "bnpl_test_key_that_is_never_logged"

type fakeTerminalAuthenticator struct {
	principal merchantterminal.MerchantPrincipal
	terminal  model.TerminalID
	apiKey    string
	calls     int
}

func (f *fakeTerminalAuthenticator) Authenticate(
	_ context.Context,
	terminalID model.TerminalID,
	apiKey string,
) (merchantterminal.MerchantPrincipal, error) {
	f.calls++

	if terminalID != f.terminal || apiKey != f.apiKey {
		return merchantterminal.MerchantPrincipal{}, cerr.NewPublic(
			cerr.CodeUnauthorized,
			"error.invalid_terminal_credentials",
			nil,
			cerr.Details{Fields: []string{}, Violations: []cerr.Violation{}},
		)
	}

	return f.principal, nil
}

func newPartnerEngine(t *testing.T, auth middleware.TerminalAuthenticator) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := newLocalizedEngine(t)
	authMW := middleware.AuthContext()

	partner := engine.Group("/partner")
	partner.Use(middleware.PartnerTerminalAuthWith(auth))
	partner.GET(
		"/whoami",
		authMW.RequireAny(permission.CheckoutSessionsCreateMerchant),
		func(c *gin.Context) {
			principal, ok := merchantterminal.MerchantPrincipalFromContext(c.Request.Context())
			require.True(t, ok)

			authCtx, ok := permission.FromContext(c.Request.Context())
			require.True(t, ok)
			require.Equal(t, model.SubjectID(principal.MerchantID), authCtx.SubjectID)
			require.True(t, authCtx.TenantID.IsZero())
			require.True(t, authCtx.ContextID.IsZero())
			require.Equal(t, principal.Permissions, authCtx.Permissions)

			c.JSON(http.StatusOK, gin.H{"merchant_id": principal.MerchantID.String()})
		},
	)

	return engine
}

func TestPartnerTerminalAuth_ValidCredentialsAttachPrincipal(t *testing.T) {
	merchantID := model.MerchantID(uuid.New())
	fake := &fakeTerminalAuthenticator{
		terminal: 4242,
		apiKey:   testTerminalAPIKey,
		principal: merchantterminal.MerchantPrincipal{
			MerchantID:  merchantID,
			TerminalID:  4242,
			Permissions: merchantterminal.InitialPartnerTerminalPermissions(),
		},
	}
	engine := newPartnerEngine(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/partner/whoami", nil)
	req.Header.Set(header.TerminalID, "4242")
	req.Header.Set(header.APIKey, testTerminalAPIKey)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), merchantID.String())
	require.Equal(t, 1, fake.calls)
}

func TestPartnerTerminalAuth_RejectionsShareOnePublicShape(t *testing.T) {
	fake := &fakeTerminalAuthenticator{
		terminal: 4242,
		apiKey:   testTerminalAPIKey,
		principal: merchantterminal.MerchantPrincipal{
			MerchantID:  model.MerchantID(uuid.New()),
			TerminalID:  4242,
			Permissions: merchantterminal.InitialPartnerTerminalPermissions(),
		},
	}

	cases := []struct {
		name         string
		terminalID   string
		apiKey       string
		authCalls    int
		extraHeaders map[string]string
		dupTerminal  bool
		dupAPIKey    bool
	}{
		{name: "missing both"},
		{name: "missing api key", terminalID: "4242"},
		{name: "missing terminal id", apiKey: testTerminalAPIKey},
		{name: "malformed terminal id", terminalID: "not-a-number", apiKey: testTerminalAPIKey},
		{name: "non positive terminal id", terminalID: "-5", apiKey: testTerminalAPIKey},
		{name: "zero terminal id", terminalID: "0", apiKey: testTerminalAPIKey},
		{name: "leading plus terminal id", terminalID: "+4242", apiKey: testTerminalAPIKey},
		{name: "leading zero terminal id", terminalID: "04242", apiKey: testTerminalAPIKey},
		{name: "whitespace terminal id", terminalID: " 4242 ", apiKey: testTerminalAPIKey},
		{
			name:       "whitespace api key",
			terminalID: "4242",
			apiKey:     " " + testTerminalAPIKey + " ",
			authCalls:  1,
		},
		{name: "unknown terminal", terminalID: "7", apiKey: testTerminalAPIKey, authCalls: 1},
		{name: "wrong api key", terminalID: "4242", apiKey: "wrong", authCalls: 1},
		{name: "duplicate terminal id", terminalID: "4242", apiKey: testTerminalAPIKey, dupTerminal: true},
		{name: "duplicate api key", terminalID: "4242", apiKey: testTerminalAPIKey, dupAPIKey: true},
		{
			name: "gateway headers without terminal credentials",
			extraHeaders: map[string]string{
				header.AuthSubjectID:   uuid.New().String(),
				header.AuthTenantID:    uuid.New().String(),
				header.AuthContextID:   uuid.New().String(),
				header.AuthPermissions: permission.CheckoutSessionsCreateMerchant.Code,
			},
		},
	}

	var reference serdser.ErrorResponse

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake.calls = 0
			engine := newPartnerEngine(t, fake)

			req := httptest.NewRequest(http.MethodGet, "/partner/whoami", nil)
			if tc.terminalID != "" {
				req.Header.Set(header.TerminalID, tc.terminalID)
			}

			if tc.apiKey != "" {
				req.Header.Set(header.APIKey, tc.apiKey)
			}

			if tc.dupTerminal {
				req.Header.Add(header.TerminalID, "9999")
			}

			if tc.dupAPIKey {
				req.Header.Add(header.APIKey, "other-key")
			}

			for name, value := range tc.extraHeaders {
				req.Header.Set(name, value)
			}

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.Equal(t, tc.authCalls, fake.calls)
			require.False(t, strings.Contains(recorder.Body.String(), testTerminalAPIKey))

			var body serdser.ErrorResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.NotNil(t, body.Details)
			require.Empty(t, body.Details.Fields)
			require.Empty(t, body.Details.Violations)

			if i == 0 {
				reference = body
			}

			require.Equal(t, reference.Code, body.Code)
			require.Equal(t, reference.Message, body.Message)
		})
	}
}

func TestPartnerTerminalAuth_PermissionStillEnforced(t *testing.T) {
	fake := &fakeTerminalAuthenticator{
		terminal: 4242,
		apiKey:   testTerminalAPIKey,
		principal: merchantterminal.MerchantPrincipal{
			MerchantID:  model.MerchantID(uuid.New()),
			TerminalID:  4242,
			Permissions: []string{permission.CheckoutSessionsReadMerchant.Code},
		},
	}
	engine := newPartnerEngine(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/partner/whoami", nil)
	req.Header.Set(header.TerminalID, "4242")
	req.Header.Set(header.APIKey, testTerminalAPIKey)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestPartnerTerminalAuth_ManagePermissionsFromPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		method      string
		route       string
		required    permission.Permission
		permissions []string
		wantStatus  int
	}{
		{
			name:        "webhook without grant",
			method:      http.MethodPut,
			route:       "/webhook-destination",
			required:    permission.CheckoutWebhooksManageMerchant,
			permissions: []string{permission.CheckoutSessionsCreateMerchant.Code},
			wantStatus:  http.StatusForbidden,
		},
		{
			name:     "webhook with grant",
			method:   http.MethodPut,
			route:    "/webhook-destination",
			required: permission.CheckoutWebhooksManageMerchant,
			permissions: []string{
				permission.CheckoutSessionsCreateMerchant.Code,
				permission.CheckoutWebhooksManageMerchant.Code,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "return destination without grant",
			method:      http.MethodPut,
			route:       "/return-destinations/default",
			required:    permission.CheckoutReturnDestinationsManageMerchant,
			permissions: []string{permission.CheckoutSessionsCreateMerchant.Code},
			wantStatus:  http.StatusForbidden,
		},
		{
			name:     "return destination with grant",
			method:   http.MethodPut,
			route:    "/return-destinations/default",
			required: permission.CheckoutReturnDestinationsManageMerchant,
			permissions: []string{
				permission.CheckoutSessionsCreateMerchant.Code,
				permission.CheckoutReturnDestinationsManageMerchant.Code,
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTerminalAuthenticator{
				terminal: 4242,
				apiKey:   testTerminalAPIKey,
				principal: merchantterminal.MerchantPrincipal{
					MerchantID:  model.MerchantID(uuid.New()),
					TerminalID:  4242,
					Permissions: tc.permissions,
				},
			}

			engine := newLocalizedEngine(t)
			authMW := middleware.AuthContext()
			partner := engine.Group("/partner")
			partner.Use(middleware.PartnerTerminalAuthWith(fake))
			partner.Handle(
				tc.method,
				tc.route,
				authMW.RequireAny(tc.required),
				func(c *gin.Context) { c.Status(http.StatusOK) },
			)

			req := httptest.NewRequest(tc.method, "/partner"+tc.route, nil)
			req.Header.Set(header.TerminalID, "4242")
			req.Header.Set(header.APIKey, testTerminalAPIKey)
			// Partner clients must not escalate via gateway permission headers.
			req.Header.Set(header.AuthPermissions, tc.required.Code)

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantStatus, recorder.Code)
		})
	}
}

func TestPartnerTerminalAuth_IgnoresGatewaySubjectAndPermissions(t *testing.T) {
	merchantID := model.MerchantID(uuid.New())
	spoofedSubject := uuid.New()

	fake := &fakeTerminalAuthenticator{
		terminal: 4242,
		apiKey:   testTerminalAPIKey,
		principal: merchantterminal.MerchantPrincipal{
			MerchantID:  merchantID,
			TerminalID:  4242,
			Permissions: merchantterminal.InitialPartnerTerminalPermissions(),
		},
	}

	gin.SetMode(gin.TestMode)
	engine := newLocalizedEngine(t)
	engine.GET("/partner/whoami", middleware.PartnerTerminalAuthWith(fake), func(c *gin.Context) {
		authCtx, ok := permission.FromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, model.SubjectID(merchantID), authCtx.SubjectID)
		require.True(t, authCtx.TenantID.IsZero())
		require.True(t, authCtx.ContextID.IsZero())
		require.Equal(t, fake.principal.Permissions, authCtx.Permissions)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/partner/whoami", nil)
	req.Header.Set(header.TerminalID, "4242")
	req.Header.Set(header.APIKey, testTerminalAPIKey)
	req.Header.Set(header.AuthSubjectID, spoofedSubject.String())
	req.Header.Set(header.AuthPermissions, "something:else")
	req.Header.Set(header.AuthTenantID, uuid.New().String())
	req.Header.Set(header.AuthContextID, uuid.New().String())

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestPartnerRoutes_WithoutGatewayScope_ReachPermissionCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	routes := []struct {
		name     string
		method   string
		path     string
		required permission.Permission
	}{
		{
			name:     "inquiry",
			method:   http.MethodGet,
			path:     "/checkout-sessions/" + uuid.New().String(),
			required: permission.CheckoutSessionsReadMerchant,
		},
		{
			name:     "cancel",
			method:   http.MethodPost,
			path:     "/checkout-sessions/" + uuid.New().String() + "/cancel",
			required: permission.CheckoutSessionsCreateMerchant,
		},
		{
			name:     "return destination",
			method:   http.MethodPut,
			path:     "/return-destinations/default",
			required: permission.CheckoutReturnDestinationsManageMerchant,
		},
		{
			name:     "webhook destination",
			method:   http.MethodPut,
			path:     "/webhook-destination",
			required: permission.CheckoutWebhooksManageMerchant,
		},
	}

	for _, tc := range routes {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTerminalAuthenticator{
				terminal: 4242,
				apiKey:   testTerminalAPIKey,
				principal: merchantterminal.MerchantPrincipal{
					MerchantID:  model.MerchantID(uuid.New()),
					TerminalID:  4242,
					Permissions: []string{tc.required.Code},
				},
			}

			engine := newLocalizedEngine(t)
			authMW := middleware.AuthContext()
			partner := engine.Group("/partner")
			partner.Use(middleware.PartnerTerminalAuthWith(fake))
			partner.Handle(
				tc.method,
				tc.path,
				authMW.RequireAny(tc.required),
				func(c *gin.Context) {
					authCtx, ok := permission.FromContext(c.Request.Context())
					require.True(t, ok)
					require.True(t, authCtx.TenantID.IsZero())
					require.True(t, authCtx.ContextID.IsZero())
					c.Status(http.StatusOK)
				},
			)

			req := httptest.NewRequest(tc.method, "/partner"+tc.path, nil)
			req.Header.Set(header.TerminalID, "4242")
			req.Header.Set(header.APIKey, testTerminalAPIKey)

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, 1, fake.calls)
		})
	}
}

func TestPartnerCreate_DoesNotRequireGatewayAuthHeaders(t *testing.T) {
	merchantID := model.MerchantID(uuid.New())

	fake := &fakeTerminalAuthenticator{
		terminal: 4242,
		apiKey:   testTerminalAPIKey,
		principal: merchantterminal.MerchantPrincipal{
			MerchantID:  merchantID,
			TerminalID:  4242,
			Permissions: merchantterminal.InitialPartnerTerminalPermissions(),
		},
	}

	gin.SetMode(gin.TestMode)
	engine := newLocalizedEngine(t)
	authMW := middleware.AuthContext()
	partner := engine.Group("/partner")
	partner.Use(middleware.PartnerTerminalAuthWith(fake))
	partner.POST(
		"/checkout-sessions",
		authMW.RequireAny(permission.CheckoutSessionsCreateMerchant),
		func(c *gin.Context) {
			authCtx, ok := permission.FromContext(c.Request.Context())
			require.True(t, ok)
			require.Equal(t, model.SubjectID(merchantID), authCtx.SubjectID)
			require.True(t, authCtx.TenantID.IsZero())
			require.True(t, authCtx.ContextID.IsZero())
			require.Equal(t, fake.principal.Permissions, authCtx.Permissions)
			c.Status(http.StatusCreated)
		},
	)

	t.Run("valid terminal without jwt headers", func(t *testing.T) {
		fake.calls = 0
		req := httptest.NewRequest(http.MethodPost, "/partner/checkout-sessions", nil)
		req.Header.Set(header.TerminalID, "4242")
		req.Header.Set(header.APIKey, testTerminalAPIKey)

		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusCreated, recorder.Code)
		require.Equal(t, 1, fake.calls)
	})

	t.Run("forged jwt headers do not alter terminal auth context", func(t *testing.T) {
		fake.calls = 0
		req := httptest.NewRequest(http.MethodPost, "/partner/checkout-sessions", nil)
		req.Header.Set(header.TerminalID, "4242")
		req.Header.Set(header.APIKey, testTerminalAPIKey)
		req.Header.Set(header.AuthTenantID, uuid.New().String())
		req.Header.Set(header.AuthContextID, uuid.New().String())
		req.Header.Set(header.AuthSubjectID, uuid.New().String())
		req.Header.Set(header.AuthPermissions, "spoofed:permission")

		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusCreated, recorder.Code)
		require.Equal(t, 1, fake.calls)
	})
}
