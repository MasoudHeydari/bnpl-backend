package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// Start handles POST /api/v1/checkout-sessions/{checkout_session_id}/start.
//
//	@Summary		Start hosted checkout evaluation
//	@Description	Verifies the authenticated BNPL customer owns the checkout session, then creates or replays the Decision and installment offer. External partner checkouts use the unauthenticated hosted OTP start instead.
//	@Tags			Checkout Sessions
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"					format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"		format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"		example(bnpl:checkout-sessions:start:self)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			checkout_session_id	path		string				true	"Checkout session UUID"	format(uuid)
//	@Success		200					{object}	StartSuccessSchema	"Success"
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
//	@Security		PermissionAuth
//	@Router			/api/v1/checkout-sessions/{checkout_session_id}/start [post]
func (rs *resource) Start(c *gin.Context) {
	sessionID, ok := checkoutSessionIDParam(c)
	if !ok {
		return
	}

	identitySubjectID, ok := authenticatedIdentitySubjectID(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.Start(c.Request.Context(), checkoutsessionuc.StartInput{
		CheckoutSessionID: sessionID,
		IdentitySubjectID: identitySubjectID,
	})
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutSessionStarted,
		serdser.SuccessMessageCheckoutSessionStarted,
		toStartResponse(output.Session, output.Offer),
	)
}
