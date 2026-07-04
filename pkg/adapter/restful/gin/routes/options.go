package routes

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/customercreditrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/customerrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/checkoutsessionrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/contractrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/decisionrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/healthrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/installmentpaymentrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/merchantrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/merchantterminalrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/swaggerrs"
)

// RegisterOptions groups registration options for HTTP resources.
//
// Each resource that exposes configurable registration options should add its
// own options here and receive them during route registration.
type RegisterOptions struct {
	// Health configures registration of the health resources.
	Health healthrs.RegisterOptions

	// Swagger configures registration of the Swagger UI resource.
	Swagger swaggerrs.RegisterOptions

	// CheckoutSession configures registration of merchant checkout-session resources.
	CheckoutSession checkoutsessionrs.RegisterOptions

	// Decision configures registration of checkout decision resources.
	Decision decisionrs.RegisterOptions

	// Contract configures registration of checkout contract resources.
	Contract contractrs.RegisterOptions

	// InstallmentPayment configures registration of installment payment resources.
	InstallmentPayment installmentpaymentrs.RegisterOptions

	// Merchant configures registration of merchant resources.
	Merchant merchantrs.RegisterOptions

	// MerchantTerminal configures registration of partner terminal credential resources.
	MerchantTerminal merchantterminalrs.RegisterOptions

	// Customer configures registration of customer read query resources.
	Customer customerrs.RegisterOptions

	// CustomerCredit configures registration of customer credit resources.
	CustomerCredit customercreditrs.RegisterOptions
}
