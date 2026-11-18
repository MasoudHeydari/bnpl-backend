package merchantrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

type resource struct {
	merchant      *merchantuc.UseCase
	creditProduct *merchantuc.CreditProductUseCase
}

// RegisterOptions configures merchant route registration.
type RegisterOptions struct {
	// CreditProduct enables the admin merchant credit product eligibility
	// routes. When nil those routes are not registered.
	CreditProduct *merchantuc.CreditProductUseCase
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

	rs := &resource{merchant: merchant, creditProduct: opts.CreditProduct}
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

	if opts.CreditProduct != nil {
		router.GET(
			"/admin/merchants/:merchant_profile_id/credit-products",
			authMW.RequireAny(permission.MerchantsReadAny),
			rs.ListCreditProducts,
		)
		router.PUT(
			"/admin/merchants/:merchant_profile_id/credit-products/:credit_kind",
			authMW.RequireAny(permission.MerchantsCreditProductsUpdateAny),
			rs.SetCreditProduct,
		)
	}
}
