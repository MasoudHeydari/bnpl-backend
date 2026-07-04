package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/checkoutsessionrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

const partnerRoutePrefix = "/api/v1/partner/"

type protectedRoutePermission struct {
	Method            string
	GinPath           string
	SwaggerPath       string
	Permission        permission.Permission
	PermissionExample string
}

func protectedRoutePermissions() []protectedRoutePermission {
	return []protectedRoutePermission{
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/partner/checkout-sessions",
			SwaggerPath: "/api/v1/partner/checkout-sessions",
			Permission:  permission.CheckoutSessionsCreateMerchant,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/partner/checkout-sessions/" + uuid.NewString(),
			SwaggerPath: "/api/v1/partner/checkout-sessions/{checkout_session_id}",
			Permission:  permission.CheckoutSessionsReadMerchant,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/partner/checkout-sessions/" + uuid.NewString() + "/cancel",
			SwaggerPath: "/api/v1/partner/checkout-sessions/{checkout_session_id}/cancel",
			Permission:  permission.CheckoutSessionsCreateMerchant,
		},
		{
			Method:      http.MethodPut,
			GinPath:     "/api/v1/partner/return-destinations/default",
			SwaggerPath: "/api/v1/partner/return-destinations/{destination_name}",
			Permission:  permission.CheckoutReturnDestinationsManageMerchant,
		},
		{
			Method:      http.MethodPut,
			GinPath:     "/api/v1/partner/webhook-destination",
			SwaggerPath: "/api/v1/partner/webhook-destination",
			Permission:  permission.CheckoutWebhooksManageMerchant,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/checkout-decisions",
			SwaggerPath: "/api/v1/checkout-decisions",
			Permission:  permission.DecisionsCreateSelf,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/checkout-decisions/" + uuid.NewString() + "/contracts",
			SwaggerPath: "/api/v1/checkout-decisions/{decision_id}/contracts",
			Permission:  permission.ContractsCreateSelf,
		},
		{
			Method: http.MethodPost,
			GinPath: "/api/v1/contracts/" + uuid.NewString() + "/installments/" +
				uuid.NewString() + "/pay",
			SwaggerPath: "/api/v1/contracts/{contract_id}/installments/{installment_id}/pay",
			Permission:  permission.PaymentsStartSelf,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/contracts",
			SwaggerPath: "/api/v1/contracts",
			Permission:  permission.ContractsReadSelf,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/contracts/" + uuid.NewString() + "/installments",
			SwaggerPath: "/api/v1/contracts/{contract_id}/installments",
			Permission:  permission.InstallmentsReadSelf,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/installments",
			SwaggerPath: "/api/v1/installments",
			Permission:  permission.InstallmentsReadSelf,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/payments",
			SwaggerPath: "/api/v1/payments",
			Permission:  permission.PaymentsReadSelf,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/customer-credit",
			SwaggerPath: "/api/v1/customer-credit",
			Permission:  permission.CreditLimitsReadSelf,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/contracts/" + uuid.NewString() + "/cancel",
			SwaggerPath: "/api/v1/contracts/{contract_id}/cancel",
			Permission:  permission.ContractsCancelSelf,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/customer-credit/initiate",
			SwaggerPath: "/api/v1/customer-credit/initiate",
			Permission:  permission.CreditLimitsCreateSelf,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/merchant",
			SwaggerPath: "/api/v1/merchant",
			Permission:  permission.MerchantProfileReadMerchant,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/merchant/branches",
			SwaggerPath: "/api/v1/merchant/branches",
			Permission:  permission.BranchesReadMerchant,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/merchant/sales",
			SwaggerPath: "/api/v1/merchant/sales",
			Permission:  permission.ContractsReadMerchant,
		},
		{
			Method:      http.MethodGet,
			GinPath:     "/api/v1/merchant/terminal",
			SwaggerPath: "/api/v1/merchant/terminal",
			Permission:  permission.MerchantTerminalsManageMerchant,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/merchant/terminal",
			SwaggerPath: "/api/v1/merchant/terminal",
			Permission:  permission.MerchantTerminalsManageMerchant,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/merchant/terminal/credentials/rotate",
			SwaggerPath: "/api/v1/merchant/terminal/credentials/rotate",
			Permission:  permission.MerchantTerminalsManageMerchant,
		},
		{
			Method:      http.MethodPost,
			GinPath:     "/api/v1/merchant/terminal/credentials/" + uuid.NewString() + "/retire",
			SwaggerPath: "/api/v1/merchant/terminal/credentials/{credential_id}/retire",
			Permission:  permission.MerchantTerminalsManageMerchant,
		},
	}
}

func permissionExample(route protectedRoutePermission) string {
	if route.PermissionExample != "" {
		return route.PermissionExample
	}

	return route.Permission.String()
}

// allEndpointPermissions lists the distinct permission codes used by protected
// HTTP endpoints. Shared codes (for example InstallmentsReadSelf) appear once.
func allEndpointPermissions() []permission.Permission {
	return []permission.Permission{
		permission.DecisionsCreateSelf,
		permission.DecisionsCreateOnBehalf,
		permission.ContractsCreateSelf,
		permission.PaymentsStartSelf,
		permission.ContractsReadSelf,
		permission.InstallmentsReadSelf,
		permission.PaymentsReadSelf,
		permission.CreditLimitsCreateSelf,
		permission.CreditLimitsReadSelf,
		permission.ContractsCancelSelf,
		permission.ContractsReadMerchant,
		permission.BranchesReadMerchant,
		permission.CheckoutSessionsCreateMerchant,
		permission.CheckoutSessionsReadMerchant,
		permission.CheckoutWebhooksManageMerchant,
		permission.CheckoutReturnDestinationsManageMerchant,
		permission.MerchantProfileReadMerchant,
		permission.MerchantTerminalsManageMerchant,
	}
}

func isPartnerRoute(route protectedRoutePermission) bool {
	return strings.HasPrefix(route.GinPath, partnerRoutePrefix)
}

// fakeTerminalAuthenticator authenticates every request as a fixed principal.
type fakeTerminalAuthenticator struct {
	permissions []string
}

func (f fakeTerminalAuthenticator) Authenticate(
	_ context.Context,
	terminalID model.TerminalID,
	_ string,
) (merchantterminal.MerchantPrincipal, error) {
	return merchantterminal.MerchantPrincipal{
		MerchantID:  model.MerchantID(uuid.New()),
		TerminalID:  terminalID,
		Permissions: f.permissions,
	}, nil
}

func TestRegisterRoutes_RequiresExactEndpointPermission(t *testing.T) {
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
	require.NoError(t, app.RegisterRoutes(engine))

	for _, route := range protectedRoutePermissions() {
		route := route
		if isPartnerRoute(route) {
			continue
		}

		t.Run(route.Method+" "+route.SwaggerPath, func(t *testing.T) {
			permissions := permissionsExcept(route.Permission)
			req := httptest.NewRequest(route.Method, route.GinPath, http.NoBody)
			req.Header.Set(header.AuthSubjectID, uuid.NewString())
			req.Header.Set(header.AuthTenantID, uuid.NewString())
			req.Header.Set(header.AuthContextID, uuid.NewString())
			req.Header.Set(header.AuthPermissions, strings.Join(permissions, ","))

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			require.Equalf(
				t,
				http.StatusForbidden,
				recorder.Code,
				"%s %s must reject every known endpoint permission except %s",
				route.Method,
				route.SwaggerPath,
				route.Permission.Code,
			)
		})
	}
}

func permissionsExcept(excluded permission.Permission) []string {
	permissions := make([]string, 0, len(allEndpointPermissions())-1)
	for _, candidate := range allEndpointPermissions() {
		if candidate.Code == excluded.Code {
			continue
		}

		permissions = append(permissions, candidate.Code)
	}

	return permissions
}

// TestRegisterRoutes_PartnerRoutesIgnoreGatewayHeaders asserts partner routes
// never accept gateway x-auth-* headers: without terminal credentials every
// partner route is unauthorized, even with every endpoint permission granted.
func TestRegisterRoutes_PartnerRoutesIgnoreGatewayHeaders(t *testing.T) {
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
	require.NoError(t, app.RegisterRoutes(engine))

	routes := make([]protectedRoutePermission, 0)
	for _, route := range protectedRoutePermissions() {
		if isPartnerRoute(route) {
			routes = append(routes, route)
		}
	}

	allPermissions := make([]string, 0, len(allEndpointPermissions()))
	for _, perm := range allEndpointPermissions() {
		allPermissions = append(allPermissions, perm.Code)
	}

	for _, route := range routes {
		route := route
		t.Run(route.Method+" "+route.SwaggerPath, func(t *testing.T) {
			req := httptest.NewRequest(route.Method, route.GinPath, http.NoBody)
			req.Header.Set(header.AuthSubjectID, uuid.NewString())
			req.Header.Set(header.AuthTenantID, uuid.NewString())
			req.Header.Set(header.AuthContextID, uuid.NewString())
			req.Header.Set(header.AuthPermissions, strings.Join(allPermissions, ","))

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}

// TestRegisterPartnerRoutes_RequiresExactEndpointPermission asserts each partner
// checkout route rejects a terminal that holds every known endpoint permission
// except the one the route requires.
func TestRegisterPartnerRoutes_RequiresExactEndpointPermission(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Usecases.Health.Enabled = false
	cfg.Gin.Swagger.Enabled = false

	app := testAppWithInfrastructure(t, cfg)
	usecases, err := app.NewUseCases()
	require.NoError(t, err)

	for _, route := range protectedRoutePermissions() {
		route := route
		if !isPartnerRoute(route) {
			continue
		}

		t.Run(route.Method+" "+route.SwaggerPath, func(t *testing.T) {
			engine, engineErr := cfg.NewGinEngine(app.Infrastructure.Trace, app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
			require.NoError(t, engineErr)

			authenticator := fakeTerminalAuthenticator{permissions: permissionsExcept(route.Permission)}
			partner := engine.Group("/api/v1/partner", middleware.PartnerTerminalAuthWith(authenticator))
			checkoutsessionrs.RegisterPartner(partner, usecases.CheckoutSession, checkoutsessionrs.RegisterOptions{})

			req := httptest.NewRequest(route.Method, route.GinPath, http.NoBody)
			req.Header.Set(header.TerminalID, "4829174650293817")
			req.Header.Set(header.APIKey, "test-api-key")
			req.Header.Set(header.AuthTenantID, uuid.NewString())
			req.Header.Set(header.AuthContextID, uuid.NewString())

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			require.Equalf(
				t,
				http.StatusForbidden,
				recorder.Code,
				"%s %s must reject every known endpoint permission except %s",
				route.Method,
				route.SwaggerPath,
				route.Permission.Code,
			)
		})
	}
}
