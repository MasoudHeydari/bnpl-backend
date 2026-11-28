package merchantterminalrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
)

type resource struct {
	merchantTerminal *merchantterminaluc.UseCase
}

// RegisterOptions configures merchant terminal route registration.
type RegisterOptions struct{}

// RegisterMerchant attaches merchant terminal lifecycle routes to router.
//
// router must be a /api/v1 group already guarded by
// middleware.AuthContext.Attach. Merchant identity and merchant-wide scope are
// resolved by the Gateway; the human subject is not the merchant identity. These routes manage the partner terminal and
// API keys of that merchant; they do not accept a path external_merchant_id or
// partner API-key headers.
func RegisterMerchant(
	router *gin.RouterGroup,
	merchantTerminal *merchantterminaluc.UseCase,
	_ RegisterOptions,
) {
	if merchantTerminal == nil {
		return
	}

	rs := &resource{merchantTerminal: merchantTerminal}
	authMW := middleware.AuthContext()
	requireManage := authMW.RequireAny(permission.MerchantTerminalsManageMerchant)
	workspace := authMW.RequireMerchantWorkspace()
	allOutlets := authMW.RequireAllMerchantOutlets()

	router.GET(
		"/merchant/terminal",
		requireManage,
		workspace,
		allOutlets,
		rs.Get,
	)
	router.POST(
		"/merchant/terminal",
		requireManage,
		workspace,
		allOutlets,
		rs.Provision,
	)
	router.POST(
		"/merchant/terminal/credentials/rotate",
		requireManage,
		workspace,
		allOutlets,
		rs.Rotate,
	)
	router.POST(
		"/merchant/terminal/credentials/:credential_id/retire",
		requireManage,
		workspace,
		allOutlets,
		rs.Retire,
	)
}
