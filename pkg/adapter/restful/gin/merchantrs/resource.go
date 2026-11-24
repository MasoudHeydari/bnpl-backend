package merchantrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

type resource struct {
	merchant   *merchantuc.UseCase
	creditKind *merchantuc.CreditKindUseCase
}

// RegisterOptions configures merchant route registration.
type RegisterOptions struct {
	// CreditKind enables the admin merchant credit kind eligibility
	// routes. When nil those routes are not registered.
	CreditKind *merchantuc.CreditKindUseCase
}

// Register attaches merchant routes to the API RouterGroup provided by wiring
// (typically /api/v1). Passing *gin.RouterGroup keeps this resource from
// mounting directly on the root Engine.
func Register(
	router *gin.RouterGroup,
	merchant *merchantuc.UseCase,
	opts RegisterOptions,
) {
	if merchant == nil {
		return
	}

	rs := &resource{merchant: merchant, creditKind: opts.CreditKind}
	authMW := middleware.AuthContext()
	router.GET(
		"/merchant",
		authMW.RequireAny(permission.MerchantProfileReadMerchant),
		rs.Get,
	)
	router.GET(
		"/merchant/branches",
		authMW.RequireAny(permission.BranchesReadMerchant),
		rs.ListBranches,
	)
	router.GET(
		"/merchant/sales",
		authMW.RequireAny(permission.ContractsReadMerchant),
		rs.ListSales,
	)

	if opts.CreditKind != nil {
		router.GET(
			"/admin/merchants/:merchant_profile_id/credit-kinds",
			authMW.RequireAny(permission.MerchantsReadAny),
			rs.ListCreditKinds,
		)
		router.PUT(
			"/admin/merchants/:merchant_profile_id/credit-kinds/:credit_kind",
			authMW.RequireAny(permission.MerchantsCreditProductsUpdateAny),
			rs.SetCreditKind,
		)
	}
}
