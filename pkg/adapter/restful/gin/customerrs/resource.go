package customerrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
)

type resource struct {
	customerAccount *customeraccountuc.UseCase
}

// RegisterOptions configures customer account route registration.
type RegisterOptions struct{}

// Register attaches customer account routes to the API RouterGroup provided by
// wiring (typically /api/v1). Passing *gin.RouterGroup keeps this resource from
// mounting directly on the root Engine.
func Register(
	router *gin.RouterGroup,
	customerAccount *customeraccountuc.UseCase,
	_ RegisterOptions,
) {
	if customerAccount == nil {
		return
	}

	rs := &resource{customerAccount: customerAccount}
	authMW := middleware.AuthContext()
	router.GET(
		"/contracts",
		authMW.RequireAny(permission.ContractsReadSelf),
		rs.ListActiveContracts,
	)
	router.GET(
		"/contracts/:contract_id/installments",
		authMW.RequireAny(permission.InstallmentsReadSelf),
		rs.GetContractInstallments,
	)
	router.GET(
		"/installments",
		authMW.RequireAny(permission.InstallmentsReadSelf),
		rs.ListUnpaidInstallments,
	)
	router.GET(
		"/payments",
		authMW.RequireAny(permission.PaymentsReadSelf),
		rs.ListPayments,
	)
	router.GET(
		"/repayment-overview",
		authMW.RequireAny(permission.InstallmentsReadSelf),
		rs.GetRepaymentOverview,
	)
}
