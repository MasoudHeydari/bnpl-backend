package installmentpaymentrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

type resource struct {
	installmentRepayment *installmentrepaymentuc.UseCase
}

// RegisterOptions configures installment payment route registration.
type RegisterOptions struct{}

// Register attaches installment payment routes to the API RouterGroup provided
// by wiring (typically /api/v1). Passing *gin.RouterGroup keeps this resource
// from mounting directly on the root Engine.
func Register(
	router *gin.RouterGroup,
	installmentRepayment *installmentrepaymentuc.UseCase,
	_ RegisterOptions,
) {
	if installmentRepayment == nil {
		return
	}

	rs := &resource{installmentRepayment: installmentRepayment}
	authMW := middleware.AuthContext()
	router.POST(
		"/contracts/:contract_id/installments/:installment_id/pay",
		authMW.RequireAny(permission.PaymentsStartSelf),
		rs.CreatePaymentAttempt,
	)
	router.POST(
		"/repayments/priority-pay",
		authMW.RequireAny(permission.PaymentsStartSelf),
		rs.CreatePriorityPaymentAttempt,
	)
}
