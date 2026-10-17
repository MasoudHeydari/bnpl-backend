package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
)

// TestSwaggerCoversRegisteredRoutes asserts the committed Swagger 2.0 document
// covers every application HTTP route registered through the normal bootstrap
// path, excluding infrastructure-only Swagger UI routes.
func TestSwaggerCoversRegisteredRoutes(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Usecases.Health.Enabled = true
	cfg.Gin.Swagger.Enabled = true

	app := testAppWithInfrastructure(t, cfg)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	app.UseCases = usecases

	engine, err := cfg.NewGinEngine(app.Infrastructure.Trace, app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)

	err = app.RegisterRoutes(engine)
	require.NoError(t, err)

	registered := map[string]map[string]struct{}{}
	for _, route := range engine.Routes() {
		if !isDocumentedHTTPMethod(route.Method) {
			continue
		}

		if isSwaggerInfrastructurePath(route.Path) {
			continue
		}

		swaggerPath := ginPathToSwaggerPath(route.Path)
		methods, ok := registered[swaggerPath]
		if !ok {
			methods = map[string]struct{}{}
			registered[swaggerPath] = methods
		}

		methods[strings.ToLower(route.Method)] = struct{}{}
	}

	spec := loadCommittedSwaggerSpec(t)
	require.NotEmpty(t, spec.Paths, "generated swagger paths must not be empty")

	documented := map[string]map[string]struct{}{}
	for path, item := range spec.Paths {
		methods := map[string]struct{}{}
		for method := range item {
			if !isDocumentedHTTPMethod(strings.ToUpper(method)) {
				continue
			}

			methods[strings.ToLower(method)] = struct{}{}
		}

		if len(methods) == 0 {
			continue
		}

		documented[path] = methods
	}

	for path, methods := range registered {
		docMethods, ok := documented[path]
		require.Truef(t, ok, "registered route path %q missing from swagger spec", path)
		for method := range methods {
			_, ok = docMethods[method]
			require.Truef(
				t,
				ok,
				"registered route %s %s missing from swagger spec",
				strings.ToUpper(method),
				path,
			)
		}
	}

	for path, methods := range documented {
		regMethods, ok := registered[path]
		require.Truef(
			t,
			ok,
			"swagger path %q is not registered as an application route",
			path,
		)
		for method := range methods {
			_, ok = regMethods[method]
			require.Truef(
				t,
				ok,
				"swagger operation %s %s is not a registered application route",
				strings.ToUpper(method),
				path,
			)
		}
	}
}

// TestSwaggerDocumentsTraceHeadersAndRealisticFailures asserts committed
// Swagger operations document the TraceID middleware response headers and do
// not invent dependency statuses the application does not currently emit.
func TestSwaggerDocumentsTraceHeadersAndRealisticFailures(t *testing.T) {
	t.Parallel()

	spec := loadCommittedSwaggerDetail(t)
	require.Equal(t, "2.0", spec.Swagger)

	requiredHeaders := []string{header.TraceID, header.RequestID, header.CorrelationID}
	pathsWith503 := map[string]struct{}{
		"/healthz/live":  {},
		"/healthz/ready": {},
		"/api/v1/checkout-decisions/{decision_id}/contracts": {},
		"/api/v1/contracts/{contract_id}/cancel":             {},
		"/api/v1/customer-credit/initiate":                   {},
	}

	for path, item := range spec.Paths {
		for method, op := range item {
			if !isDocumentedHTTPMethod(strings.ToUpper(method)) {
				continue
			}

			require.NotEmptyf(t, op.Responses, "%s %s must document responses", method, path)

			for status, resp := range op.Responses {
				for _, name := range requiredHeaders {
					_, ok := resp.Headers[name]
					require.Truef(
						t,
						ok,
						"%s %s response %s missing header %s",
						strings.ToUpper(method),
						path,
						status,
						name,
					)
				}

				switch status {
				case "429", "502", "504":
					t.Fatalf(
						"%s %s documents unsupported status %s",
						strings.ToUpper(method),
						path,
						status,
					)

				case "503":
					_, ok := pathsWith503[path]
					require.Truef(
						t,
						ok,
						"%s %s documents 503 but is not an endpoint that currently returns SERVICE_UNAVAILABLE",
						strings.ToUpper(method),
						path,
					)
				}
			}
		}
	}
}

type swaggerSpec struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

type swaggerDetailSpec struct {
	Swagger string                                    `json:"swagger"`
	Paths   map[string]map[string]swaggerOperationDoc `json:"paths"`
}

type swaggerOperationDoc struct {
	Parameters []swaggerParameterDoc         `json:"parameters"`
	Responses  map[string]swaggerResponseDoc `json:"responses"`
	Security   []map[string][]string         `json:"security"`
}

type swaggerParameterDoc struct {
	Name    string `json:"name"`
	In      string `json:"in"`
	Type    string `json:"type"`
	Format  string `json:"format"`
	Example string `json:"example"`
}

type swaggerResponseDoc struct {
	Headers map[string]json.RawMessage `json:"headers"`
}

// customerTokenCheckoutPathPrefix prefixes unauthenticated customer-token
// checkout routes (public token in path; no gateway auth).
const customerTokenCheckoutPathPrefix = "/api/v1/checkout-sessions/{token}"

func TestSwaggerAuthContractAndInitialCreditHasNoBody(t *testing.T) {
	t.Parallel()

	spec := loadCommittedSwaggerDetail(t)

	authHeaderNames := []string{
		header.AuthSubjectID,
		header.AuthTenantID,
		header.AuthContextID,
		header.AuthPermissions,
	}
	expectedPermissions := make(map[string]string, len(protectedRoutePermissions()))
	partnerOperations := map[string]struct{}{}
	for _, route := range protectedRoutePermissions() {
		if isPartnerRoute(route) {
			partnerOperations[strings.ToUpper(route.Method)+" "+route.SwaggerPath] = struct{}{}

			continue
		}

		key := strings.ToUpper(route.Method) + " " + route.SwaggerPath
		expectedPermissions[key] = permissionExample(route)
	}

	seenProtectedOperations := make(map[string]struct{}, len(expectedPermissions))
	for path, item := range spec.Paths {
		if strings.HasPrefix(path, "/healthz") {
			continue
		}

		for method, op := range item {
			if !isDocumentedHTTPMethod(strings.ToUpper(method)) {
				continue
			}

			operationKey := strings.ToUpper(method) + " " + path

			// Hosted external routes are unauthenticated: the public checkout
			// token and SMS OTP are the credentials, so no gateway auth applies.
			if strings.HasPrefix(path, customerTokenCheckoutPathPrefix) {
				require.Falsef(
					t,
					hasSwaggerSecurity(op.Security, "PermissionAuth") ||
						hasSwaggerSecurity(op.Security, "TerminalID"),
					"%s must not declare gateway or terminal authentication",
					operationKey,
				)

				continue
			}

			// Partner routes authenticate with terminal credentials, not
			// gateway headers.
			if _, partner := partnerOperations[operationKey]; partner {
				require.Truef(
					t,
					hasSwaggerSecurity(op.Security, "TerminalID") && hasSwaggerSecurity(op.Security, "TerminalAPIKey"),
					"%s must declare TerminalID and TerminalAPIKey",
					operationKey,
				)
				require.Falsef(
					t,
					hasSwaggerSecurity(op.Security, "PermissionAuth"),
					"%s must not declare gateway PermissionAuth",
					operationKey,
				)

				continue
			}

			paramsByName := map[string]swaggerParameterDoc{}
			for _, param := range op.Parameters {
				if param.In == "header" {
					paramsByName[param.Name] = param
				}
			}

			expectedPermission, ok := expectedPermissions[operationKey]
			require.Truef(t, ok, "%s is missing from the protected-route permission contract", operationKey)
			seenProtectedOperations[operationKey] = struct{}{}

			for _, name := range authHeaderNames {
				param, ok := paramsByName[name]
				require.Truef(t, ok, "%s missing auth header %s", operationKey, name)
				if name != header.AuthPermissions {
					require.Equalf(t, "uuid", param.Format, "%s header %s must be uuid format", operationKey, name)
				}
			}

			permissionParam := paramsByName[header.AuthPermissions]
			require.Equalf(
				t,
				expectedPermission,
				permissionParam.Example,
				"%s must document the same permission enforced by route registration",
				operationKey,
			)
			require.Truef(
				t,
				hasSwaggerSecurity(op.Security, "PermissionAuth"),
				"%s must declare PermissionAuth",
				operationKey,
			)

			if path == "/api/v1/customer-credit/initiate" {
				for _, param := range op.Parameters {
					require.NotEqualf(t, "body", param.In, "initial credit must not document a body parameter")
				}
			}
		}
	}

	require.Len(t, seenProtectedOperations, len(expectedPermissions))
}

func hasSwaggerSecurity(security []map[string][]string, name string) bool {
	for _, requirement := range security {
		if _, ok := requirement[name]; ok {
			return true
		}
	}

	return false
}

func loadCommittedSwaggerSpec(t *testing.T) swaggerSpec {
	t.Helper()

	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "gen", "swagger", "swagger.json"))
	require.NoError(t, err)

	var spec swaggerSpec
	require.NoError(t, json.Unmarshal(data, &spec))
	return spec
}

func loadCommittedSwaggerDetail(t *testing.T) swaggerDetailSpec {
	t.Helper()

	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "gen", "swagger", "swagger.json"))
	require.NoError(t, err)

	var spec swaggerDetailSpec
	require.NoError(t, json.Unmarshal(data, &spec))
	return spec
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found from %s", dir)
		dir = parent
	}
}

func ginPathToSwaggerPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}

	return strings.Join(parts, "/")
}

func isSwaggerInfrastructurePath(path string) bool {
	return strings.HasPrefix(path, "/swagger")
}

func isDocumentedHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true

	default:
		return false
	}
}
