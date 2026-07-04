package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// CreatePartner handles POST /api/v1/partner/checkout-sessions.
//
//	@Summary		Create external partner checkout session
//	@Description	Creates an idempotent merchant checkout handoff and prepares the financing offer internally.
//	@Description	No customer identity, credit balance, Decision, or installment offer is disclosed to the Merchant.
//	@Description	Personalized evaluation is prepared after BNPL resolves the Merchant-asserted phone through Identity. The offer is presented read-only in Hosted GET. OTP verification later proves phone possession and authorizes confirmation of that exact offer generation.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			X-Terminal-Id		header		string						true	"Public numeric terminal identifier of the partner"
//	@Param			X-API-Key			header		string						true	"Secret API key of the partner terminal"
//	@Param			X-Auth-Tenant-Id	header		string						true	"Gateway-injected tenant UUID (NOT merchant-selected; NOT a request-body field). Required only on Partner Create. Production infrastructure must strip client-supplied X-Auth-* and inject authoritative values after token validation; this BNPL repository does not enforce that Gateway trust boundary."
//	@Param			X-Auth-Context-Id	header		string						true	"Gateway-injected context UUID (NOT merchant-selected; NOT a request-body field). Required only on Partner Create. Production infrastructure must strip client-supplied X-Auth-* and inject authoritative values after token validation; this BNPL repository does not enforce that Gateway trust boundary."
//	@Param			X-Trace-Id			header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			body				body		CreatePartnerRequestSchema	true	"Partner checkout session request"
//	@Success		201					{object}	PartnerCreateSuccessSchema	"Checkout session created"
//	@Success		200					{object}	PartnerCreateSuccessSchema	"Idempotent replay of an existing checkout session"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			200					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			201					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			201					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			201					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		TerminalID && TerminalAPIKey
//	@Router			/api/v1/partner/checkout-sessions [post]
func (rs *resource) CreatePartner(c *gin.Context) {
	input, ok := toCreatePartnerInput(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.CreatePartner(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	status := http.StatusCreated
	if output.Result == checkoutsessionuc.CreateResultAlreadyExists {
		status = http.StatusOK
	}

	serdser.Ser(
		c,
		status,
		serdser.SuccessCodeCheckoutSessionCreated,
		serdser.SuccessMessageCheckoutSessionCreated,
		toPartnerCreateResponse(output),
	)
}
