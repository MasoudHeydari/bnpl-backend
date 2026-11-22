package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// VerifyByMerchant handles POST /api/v1/partner/checkout-sessions/{checkout_session_id}/verify.
//
//	@Summary		Verify partner checkout session
//	@Description	Records explicit merchant acceptance after durable Financial Core success (status verify_pending).
//	@Description	Idempotent: repeating Verify for an already verified checkout owned by the same merchant with a matching merchant_order_id returns the stored verification receipt.
//	@Description	Never activates contracts, releases credit, or treats browser redirects as payment confirmation.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			X-Terminal-Id		header		string						true	"Public numeric terminal identifier of the partner"
//	@Param			X-API-Key			header		string						true	"Secret API key of the partner terminal"
//	@Param			X-Trace-Id			header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			checkout_session_id	path		string						true	"Public checkout session token (cs_<64 hex>)"
//	@Param			body				body		VerifyByMerchantRequestSchema	true	"Merchant verification request"
//	@Success		200					{object}	VerifyByMerchantSuccessSchema	"Checkout verified by merchant"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			200					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		TerminalID && TerminalAPIKey
//	@Router			/api/v1/partner/checkout-sessions/{checkout_session_id}/verify [post]
func (rs *resource) VerifyByMerchant(c *gin.Context) {
	input, ok := toVerifyByMerchantInput(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.VerifyByMerchant(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutSessionVerified,
		serdser.SuccessMessageCheckoutSessionVerified,
		toVerifyByMerchantResponse(output),
	)
}
