package routes

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/checkoutsessionrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/contractrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/customercreditrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/customerrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/decisionrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/healthrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/installmentpaymentrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/merchantrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/merchantterminalrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/swaggerrs"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/healthcheckuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

// Register attaches HTTP routes to router.
//
// Operational Prometheus scraping is intentionally not registered on the
// application engine; see the dedicated metrics HTTP server in bootstrap.
//
// router is typically the root *gin.Engine from bootstrap (accepted as
// gin.IRouter so health/swagger can mount on the same surface). Scoped API
// resources receive /api/v1 *gin.RouterGroup values created here — they must
// not register on the root Engine directly.
//
// /api/v1 is split into three authentication surfaces:
//   - /api/v1/partner authenticates Merchant identity with PartnerTerminalAuth
//     (X-Terminal-Id and X-API-Key). Financial Core TenantID/ContextID for
//     Offer preparation mode comes from trusted MerchantProfile configuration.
//     Gateway-provided SubjectID/Permissions never control Partner Merchant
//     identity or permissions. The full AuthContext.Attach chain is not applied.
//   - /api/v1/checkout-sessions/:token is the unauthenticated customer checkout
//     surface (sibling of AuthContext.Attach, not nested under it). The public
//     checkout token and SMS OTP are the credentials.
//   - every other /api/v1 route is guarded by the gateway AuthContext.Attach
//     (subject, tenant, context, and permissions), including merchant terminal
//     provision, rotate, and retire.
//
// Partner routes are registered on their own group, so the full gateway Attach
// middleware is never applied to them.
func Register(
	router gin.IRouter,
	healthUseCase *healthcheckuc.UseCase,
	checkoutSessionUseCase *checkoutsessionuc.UseCase,
	decisionUseCase *decisionuc.UseCase,
	installmentRepaymentUseCase *installmentrepaymentuc.UseCase,
	contractUseCase *contractuc.UseCase,
	merchantUseCase *merchantuc.UseCase,
	customerAccountUseCase *customeraccountuc.UseCase,
	customerCreditUseCase *customercredituc.UseCase,
	merchantTerminalUseCase *merchantterminaluc.UseCase,
	opts RegisterOptions,
) error {
	const op = "routes.Register"

	if merchantTerminalUseCase == nil {
		return fmt.Errorf("%s: merchant terminal use case is required", op)
	}

	healthrs.Register(router, healthUseCase, opts.Health)
	swaggerrs.Register(router, opts.Swagger)

	api := router.Group("/api/v1")

	partner := api.Group("/partner", middleware.PartnerTerminalAuth(merchantTerminalUseCase))
	checkoutsessionrs.RegisterPartner(partner, checkoutSessionUseCase, opts.CheckoutSession)

	checkoutsessionrs.RegisterCustomerToken(api, checkoutSessionUseCase, opts.CheckoutSession)

	authContext := middleware.AuthContext()
	v1 := api.Group("", authContext.Attach())

	merchantterminalrs.RegisterMerchant(v1, merchantTerminalUseCase, opts.MerchantTerminal)
	decisionrs.Register(v1, decisionUseCase, opts.Decision)
	contractrs.Register(v1, contractUseCase, opts.Contract)
	merchantrs.Register(v1, merchantUseCase, opts.Merchant)
	installmentpaymentrs.Register(v1, installmentRepaymentUseCase, opts.InstallmentPayment)
	customerrs.Register(v1, customerAccountUseCase, opts.Customer)
	customercreditrs.Register(v1, customerCreditUseCase, opts.CustomerCredit)

	return nil
}
