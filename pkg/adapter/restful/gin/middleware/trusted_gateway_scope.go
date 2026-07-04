package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

// TrustedGatewayScope attaches Gateway-injected TenantID and ContextID onto an
// existing AuthContext without altering SubjectID or Permissions.
//
// Use only on routes that snapshot Financial Core request scope (primarily
// Partner Create Checkout). PartnerTerminalAuth authenticates Merchant identity
// from terminal credentials alone; this middleware is the second, independent
// trust source for TenantID/ContextID.
//
// X-Auth-* values are trusted only because the Gateway strips client-supplied
// headers and injects authoritative ones after validating the caller's token.
// BNPL never parses JWTs and never accepts tenant/context from JSON/query/path.
// Gateway-provided X-Auth-Subject-Id / X-Auth-Permissions are ignored here —
// Merchant identity and permissions remain terminal-owned.
func TrustedGatewayScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, ok := permission.FromContext(c.Request.Context())
		if !ok {
			serdser.SerErr(c, authContextMissingError())
			c.Abort()

			return
		}

		tenantID, err := requireTrustedAuthUUIDHeader(c, header.AuthTenantID)
		if err != nil {
			serdser.SerErr(c, err)
			c.Abort()

			return
		}

		contextID, err := requireTrustedAuthUUIDHeader(c, header.AuthContextID)
		if err != nil {
			serdser.SerErr(c, err)
			c.Abort()

			return
		}

		// Preserve terminal-derived SubjectID and Permissions; only fill scope.
		authCtx.TenantID = model.TenantID(tenantID)
		authCtx.ContextID = model.ContextID(contextID)

		ctx := permission.WithAuthContext(c.Request.Context(), authCtx)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
