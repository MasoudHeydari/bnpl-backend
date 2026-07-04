package customercreditrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

type resource struct {
	customerCredit *customercredituc.UseCase
}

// RegisterOptions configures customer credit route registration.
type RegisterOptions struct{}

// Register attaches customer credit routes to the API RouterGroup provided by
// wiring (typically /api/v1). Passing *gin.RouterGroup keeps this resource from
// mounting directly on the root Engine.
func Register(
	router *gin.RouterGroup,
	customerCredit *customercredituc.UseCase,
	_ RegisterOptions,
) {
	if customerCredit == nil {
		return
	}

	rs := &resource{customerCredit: customerCredit}
	authMW := middleware.AuthContext()
	router.GET(
		"/customer-credit",
		authMW.RequireAny(permission.CreditLimitsReadSelf),
		rs.Get,
	)
	router.POST(
		"/customer-credit/initiate",
		authMW.RequireAny(permission.CreditLimitsCreateSelf),
		rs.AssignInitialLimit,
	)
}
