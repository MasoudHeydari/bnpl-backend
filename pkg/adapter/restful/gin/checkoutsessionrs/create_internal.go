package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// CreateInternal handles POST /api/v1/internal/checkout-sessions.
//
//	@Summary		Create trusted internal checkout session
//	@Description	Creates the checkout and immediately evaluates the authenticated on-behalf customer.
//	@Description	Returns checkout_url plus BNPL available credit and persisted installment-offer details to trusted first-party callers.
//	@Description	Final customer confirmation still happens on the hosted BNPL page; this response is not acceptance of terms.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			x-auth-subject-id	header		string						true	"Stable authenticated subject identifier"	format(uuid)
//	@Param			x-auth-tenant-id	header		string						true	"Authenticated tenant UUID"					format(uuid)
//	@Param			x-auth-context-id	header		string						true	"Authenticated business-context UUID"		format(uuid)
//	@Param			x-auth-permissions	header		string						true	"Comma-separated gateway permissions"		example(bnpl:checkout-sessions:create:merchant,bnpl:decisions:create:on-behalf)
//	@Param			X-Trace-Id			header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			body				body		CreateInternalRequestSchema	true	"Internal checkout session request"
//	@Success		201					{object}	InternalCreateSuccessSchema	"Checkout session created and evaluated"
//	@Success		200					{object}	InternalCreateSuccessSchema	"Idempotent replay of an existing checkout session"
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
//	@Security		PermissionAuth
//	@Router			/api/v1/internal/checkout-sessions [post]
func (rs *resource) CreateInternal(c *gin.Context) {
	input, ok := toCreateInternalInput(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.CreateInternal(c.Request.Context(), input)
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
		toInternalCreateResponse(output),
	)
}
