package decisionrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

type resource struct {
	decision *decisionuc.UseCase
}

// RegisterOptions configures checkout decision route registration.
type RegisterOptions struct{}

// Register attaches checkout decision routes to the API RouterGroup provided by
// wiring (typically /api/v1). Passing *gin.RouterGroup keeps this resource from
// mounting directly on the root Engine.
func Register(router *gin.RouterGroup, decision *decisionuc.UseCase, _ RegisterOptions) {
	if decision == nil {
		return
	}

	rs := &resource{decision: decision}
	authMW := middleware.AuthContext()
	router.POST(
		"/checkout-decisions",
		authMW.RequireAny(permission.DecisionsCreateSelf),
		rs.Create,
	)
}
