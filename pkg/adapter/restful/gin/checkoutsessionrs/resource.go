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

// RegisterPartner attaches external partner checkout routes to router.
//
// router must be the /api/v1/partner group already guarded by
// middleware.PartnerTerminalAuth (terminal credentials only). Merchant identity
// and permissions come from the authenticated terminal principal.
// X-Auth-Subject-Id and X-Auth-Permissions are never used for Merchant
// identity on these routes.
//
// Partner Create additionally chains TrustedGatewayScope so Gateway-injected
// X-Auth-Tenant-Id / X-Auth-Context-Id are snapshotted onto the new
// CheckoutSession for later Financial Core fulfillment. Other Partner routes
// do not require that scope.
//
// Integration-config routes (webhook, return destinations)
// each require their own manage permission. Those grants come from
// server-side merchant_terminal_permissions loaded at Authenticate — not from
// the API key, partner headers, or a runtime hard-coded default. Provision
// seeds the supported partner integration set; operators may trim assignments
// later without rotating credentials.
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
		middleware.TrustedGatewayScope(),
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

// RegisterHostedExternal attaches the unauthenticated hosted external checkout
// routes to router.
//
// router must be a group that is NOT guarded by AuthContext or partner
// terminal auth: the customer's browser has no gateway identity. The public
// checkout token is an unguessable locator for the checkout context; possession
// of it does not authorize contract creation. External transaction
// authorization requires successful checkout-scoped OTP verification for the
// current evaluation_generation. Payment TenantID/ContextID come from the
// immutable scope snapshotted at Partner Create. Internal sessions are
// indistinguishable from missing ones on these routes.
//
// Responses set Cache-Control: no-store and Referrer-Policy: no-referrer so the
// cs_ token is not cached or leaked via Referer. The Hosted BNPL frontend HTML
// should send the same headers.
func RegisterHostedExternal(
	router *gin.RouterGroup,
	checkoutSession *checkoutsessionuc.UseCase,
	_ RegisterOptions,
) {
	if checkoutSession == nil {
		return
	}

	rs := &resource{checkoutSession: checkoutSession}
	router.Use(middleware.HostedExternalHardening())

	router.GET("/checkout-sessions/:checkout_session_id", rs.GetHostedExternal)
	router.POST("/checkout-sessions/:checkout_session_id/start", rs.StartHostedExternal)
	router.POST("/checkout-sessions/:checkout_session_id/confirm", rs.ConfirmHostedExternal)
	router.GET("/checkout-sessions/:checkout_session_id/return", rs.ReturnHostedExternal)
}

// RegisterHostedInternal attaches the trusted internal create route and the
// hosted customer routes to router.
//
// router must be the /api/v1 group guarded by the gateway AuthContext.Attach
// middleware.
//
// The internal create route requires both checkout creation and the on-behalf
// decision capability. Either capability alone is insufficient because the route
// lets a caller evaluate financing for a customer who is not the authenticated
// subject.
func RegisterHostedInternal(
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
		"/internal/checkout-sessions",
		authMW.RequireAll(
			permission.CheckoutSessionsCreateMerchant,
			permission.DecisionsCreateOnBehalf,
		),
		rs.CreateInternal,
	)
	router.GET(
		"/checkout-sessions/:checkout_session_id",
		authMW.RequireAny(permission.CheckoutSessionsReadSelf),
		rs.Get,
	)
	router.POST(
		"/checkout-sessions/:checkout_session_id/start",
		authMW.RequireAny(permission.CheckoutSessionsStartSelf),
		rs.Start,
	)
	router.POST(
		"/checkout-sessions/:checkout_session_id/confirm",
		authMW.RequireAny(permission.CheckoutSessionsStartSelf),
		rs.Confirm,
	)
	router.GET(
		"/checkout-sessions/:checkout_session_id/return",
		authMW.RequireAny(permission.CheckoutSessionsReadSelf),
		rs.GetReturn,
	)
	router.POST(
		"/checkout-sessions/:checkout_session_id/cancel",
		authMW.RequireAny(permission.CheckoutSessionsStartSelf),
		rs.CancelByCustomer,
	)
}
