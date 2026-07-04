package contractrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

type resource struct {
	contract *contractuc.UseCase
}

// RegisterOptions configures contract route registration.
type RegisterOptions struct{}

// Register attaches contract routes to the API RouterGroup provided by wiring
// (typically /api/v1). Passing *gin.RouterGroup keeps this resource from
// mounting directly on the root Engine.
func Register(
	router *gin.RouterGroup,
	contract *contractuc.UseCase,
	_ RegisterOptions,
) {
	if contract == nil {
		return
	}

	rs := &resource{contract: contract}
	authMW := middleware.AuthContext()
	router.POST(
		"/checkout-decisions/:decision_id/contracts",
		authMW.RequireAny(permission.ContractsCreateSelf),
		rs.CreateContract,
	)
	router.POST(
		"/contracts/:contract_id/cancel",
		authMW.RequireAny(permission.ContractsCancelSelf),
		rs.Cancel,
	)
}
