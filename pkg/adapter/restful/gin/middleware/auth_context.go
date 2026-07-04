package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

// AuthContext returns middleware helpers for gateway authentication-context
// extraction and permission authorization.
func AuthContext() AuthContextMiddleware {
	return AuthContextMiddleware{}
}

// AuthContextMiddleware groups authentication extraction and permission checks.
type AuthContextMiddleware struct{}

// Attach extracts trusted authentication headers into request context.
//
// X-Auth-* headers are trusted only because the Gateway strips client-supplied
// values and injects values derived from the authenticated token. BNPL never
// parses JWTs itself. Missing, duplicate, or malformed authentication metadata
// is serialized as cerr.Unauthorized with field-oriented public details.
func (AuthContextMiddleware) Attach() gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, err := authContextFromHeaders(c)
		if err != nil {
			serdser.SerErr(c, err)
			c.Abort()

			return
		}

		ctx := permission.WithAuthContext(c.Request.Context(), authCtx)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequireAny checks that the authenticated context includes at least one of the
// expected permissions.
func (AuthContextMiddleware) RequireAny(expected ...permission.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, ok := permission.FromContext(c.Request.Context())
		if !ok {
			serdser.SerErr(c, authContextMissingError())
			c.Abort()
			return
		}

		if !authCtx.HasAnyPermission(expected...) {
			serdser.SerErr(c, permissionForbiddenError(permissionsToStrings(expected)))
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAll checks that the authenticated context includes every expected
// permission. It is used only where a route crosses two independent trust
// boundaries and possessing either capability alone is insufficient.
func (AuthContextMiddleware) RequireAll(expected ...permission.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, ok := permission.FromContext(c.Request.Context())
		if !ok {
			serdser.SerErr(c, authContextMissingError())
			c.Abort()
			return
		}

		for _, required := range expected {
			if !authCtx.HasAnyPermission(required) {
				serdser.SerErr(c, permissionForbiddenError(permissionsToStrings(expected)))
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

func authContextFromHeaders(c *gin.Context) (permission.AuthContext, error) {
	subjectID, err := requireTrustedAuthUUIDHeader(c, header.AuthSubjectID)
	if err != nil {
		return permission.AuthContext{}, err
	}

	tenantID, err := requireTrustedAuthUUIDHeader(c, header.AuthTenantID)
	if err != nil {
		return permission.AuthContext{}, err
	}

	contextID, err := requireTrustedAuthUUIDHeader(c, header.AuthContextID)
	if err != nil {
		return permission.AuthContext{}, err
	}

	rawPermissions, ok := singleHeaderValue(c, header.AuthPermissions)
	if !ok {
		return permission.AuthContext{}, authHeaderRequiredError(header.AuthPermissions)
	}

	permissions, err := permissionsFromHeader(rawPermissions)
	if err != nil {
		return permission.AuthContext{}, err
	}

	return permission.AuthContext{
		SubjectID:   model.SubjectID(subjectID),
		TenantID:    model.TenantID(tenantID),
		ContextID:   model.ContextID(contextID),
		Permissions: permissions,
	}, nil
}

func permissionsFromHeader(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, authHeaderRequiredError(header.AuthPermissions)
	}

	parts := strings.Split(value, ",")
	permissions := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		if _, ok := seen[trimmed]; ok {
			continue
		}

		seen[trimmed] = struct{}{}
		permissions = append(permissions, trimmed)
	}

	if len(permissions) == 0 {
		return nil, authHeaderRequiredError(header.AuthPermissions)
	}

	return permissions, nil
}

func permissionsToStrings(permissions []permission.Permission) []string {
	values := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		values = append(values, permission.String())
	}

	return values
}

// authContextMissingError covers RequireAny/RequireAll running without Attach
// having populated authentication metadata.
func authContextMissingError() error {
	return cerr.NewPublic(
		cerr.CodeUnauthorized,
		cerr.DefaultMessageKey(cerr.CodeUnauthorized),
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.authentication_context_missing",
					Fields:     []string{},
				},
			},
		},
	)
}

// permissionForbiddenError is returned when the caller is authenticated but
// does not satisfy the required permission check (any-of or all-of). Required
// permission names stay out of Fields (they are not request fields); they are
// used only for internal diagnostics.
func permissionForbiddenError(required []string) error {
	const op = "middleware.permissionForbiddenError"

	return cerr.NewPublic(
		cerr.CodeForbidden,
		cerr.DefaultMessageKey(cerr.CodeForbidden),
		fmt.Errorf("%s: caller does not satisfy required permissions: %v", op, required),
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "FORBIDDEN",
					MessageKey: "error.default.forbidden",
					Fields:     []string{},
				},
			},
		},
	)
}
