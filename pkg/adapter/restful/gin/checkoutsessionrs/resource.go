package checkoutsessionrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

type resource struct {
	checkoutSession *checkoutsessionuc.UseCase
}

// RegisterOptions configures merchant checkout-session route registration.
type RegisterOptions struct{}

// RegisterPartner attaches merchant server-to-server checkout routes to router.
//
// SECURITY BOUNDARY: this registration is separated from RegisterCustomerToken
// because merchant requests authenticate with terminal credentials and carry
// merchant-scoped permissions. The split is about caller trust/authentication,
// NOT internal versus external merchant classification. All merchants use this
// terminal-authenticated surface regardless of their business classification.
// Do not merge the route groups or apply customer-token trust rules here.
//
// router must be the /api/v1/partner group already guarded by
// middleware.PartnerTerminalAuth (terminal credentials only). Merchant identity
// and permissions come from the authenticated terminal principal.
// X-Auth-Subject-Id, X-Auth-Tenant-Id, X-Auth-Context-Id, and
// X-Auth-Permissions are never required and never trusted for Merchant
// identity or permissions on these routes.
func RegisterPartner(
	router *gin.RouterGroup,
	checkoutSession *checkoutsessionuc.UseCase,
	_ RegisterOptions,
) {
	if checkoutSession == nil {
		return
	}

	rs := &resource{checkoutSession: checkoutSession}
	authMW := middleware.AuthContext()

	router.POST(
		"/checkout-sessions",
		authMW.RequireAny(permission.CheckoutSessionsCreateMerchant),
		rs.CreatePartner,
	)
	router.GET(
		"/checkout-sessions/:checkout_session_id",
		authMW.RequireAny(permission.CheckoutSessionsReadMerchant),
		rs.GetPartnerStatus,
	)
	router.POST(
		"/checkout-sessions/:checkout_session_id/cancel",
		authMW.RequireAny(permission.CheckoutSessionsCreateMerchant),
		rs.CancelByMerchant,
	)
	router.PUT(
		"/return-destinations/:destination_name",
		authMW.RequireAny(permission.CheckoutReturnDestinationsManageMerchant),
		rs.UpsertReturnDestination,
	)
	router.PUT(
		"/webhook-destination",
		authMW.RequireAny(permission.CheckoutWebhooksManageMerchant),
		rs.UpsertWebhookDestination,
	)
}

// SECURITY BOUNDARY: hosted browser routes must remain separate from the
// terminal-authenticated merchant routes. The public token locates a checkout;
// it does not authenticate a merchant or authorize purchase confirmation.
// Confirmation requires the checkout-bound, single-use OTP. This distinction
// is independent of whether the merchant is internal or external.
//
// RegisterCustomerToken attaches the unauthenticated customer checkout routes to
// router as a sibling of the gateway AuthContext.Attach group.
//
// router must be the /api/v1 group without AuthContext.Attach. The public
// checkout token is an unguessable locator; OTP verification authorizes
// confirmation. These routes never require JWT tenant/context headers.
func RegisterCustomerToken(
	router *gin.RouterGroup,
	checkoutSession *checkoutsessionuc.UseCase,
	_ RegisterOptions,
) {
	if checkoutSession == nil {
		return
	}

	rs := &resource{checkoutSession: checkoutSession}
	group := router.Group("/checkout-sessions")
	group.Use(middleware.HostedExternalHardening())

	group.GET("/:token", rs.GetHostedExternal)
	group.POST("/:token/start", rs.StartHostedExternal)
	group.POST("/:token/otp", rs.RequestHostedExternalOTP)
	group.POST("/:token/confirm", rs.ConfirmHostedExternal)
	group.GET("/:token/confirmation-status", rs.GetHostedConfirmationStatus)
	group.GET("/:token/return", rs.ReturnHostedExternal)
	group.POST("/:token/cancel", rs.CancelByToken)
}
