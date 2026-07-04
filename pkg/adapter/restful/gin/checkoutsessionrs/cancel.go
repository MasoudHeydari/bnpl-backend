package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// CancelByMerchant handles POST /api/v1/partner/checkout-sessions/{checkout_session_id}/cancel.
//
//	@Summary		Cancel partner checkout session
//	@Description	Idempotently cancels an open checkout owned by the merchant. A checkout with submitted or ambiguous payment cannot be canceled until Financial Core resolves it.
//	@Tags			Checkout Sessions
//	@Produce		json
//	@Param			X-Terminal-Id		header		string				true	"Public numeric terminal identifier of the partner"
//	@Param			X-API-Key			header		string				true	"Secret API key of the partner terminal"
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			checkout_session_id	path		string				true	"Public checkout session token (cs_<64 hex>)"
//	@Success		200					{object}	CancelSuccessSchema	"Success"
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
//	@Router			/api/v1/partner/checkout-sessions/{checkout_session_id}/cancel [post]
func (rs *resource) CancelByMerchant(c *gin.Context) {
	token, ok := checkoutSessionTokenParam(c)
	if !ok {
		return
	}

	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.CancelByMerchant(
		c.Request.Context(),
		checkoutsessionuc.CancelByMerchantInput{
			CheckoutSessionToken: token,
			MerchantID:           merchantID,
		},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serCancel(c, output)
}

// CancelByCustomer handles POST /api/v1/checkout-sessions/{checkout_session_id}/cancel.
//
//	@Summary		Cancel hosted checkout
//	@Description	Cancels the checkout for the authenticated bound customer when no submitted or ambiguous payment prevents cancellation.
//	@Description	Returns the pre-registered merchant return URL with checkout_session_id and merchant state.
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
//	@Success		200					{object}	CancelSuccessSchema	"Success"
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
//	@Router			/api/v1/checkout-sessions/{checkout_session_id}/cancel [post]
func (rs *resource) CancelByCustomer(c *gin.Context) {
	sessionID, ok := checkoutSessionIDParam(c)
	if !ok {
		return
	}

	identitySubjectID, ok := authenticatedIdentitySubjectID(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.CancelByCustomer(
		c.Request.Context(),
		checkoutsessionuc.CancelByCustomerInput{
			CheckoutSessionID: sessionID,
			IdentitySubjectID: identitySubjectID,
		},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serCancel(c, output)
}

func serCancel(c *gin.Context, output checkoutsessionuc.CancelOutput) {
	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutSessionCanceled,
		serdser.SuccessMessageCheckoutSessionCanceled,
		toCancelResponse(output),
	)
}
