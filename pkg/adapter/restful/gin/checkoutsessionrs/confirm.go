package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Confirm handles POST /api/v1/checkout-sessions/{checkout_session_id}/confirm.
//
//	@Summary		Confirm hosted checkout and create contract
//	@Description	Confirms the currently displayed BNPL offer version (evaluation_generation) and, when customer action is required, returns the redirect URL for the next step.
//	@Description	Clients never supply decision_id, contract_id, or payment provider selection; Confirm reads session state and routes payment internally.
//	@Description	Flow: OPEN -> CONFIRMING (atomic claim) -> PROCESSING (payment pending) -> COMPLETED. Cancel and expiry are refused while CONFIRMING or PROCESSING.
//	@Description	When evaluation_generation is stale the offer is re-evaluated and the response has reconfirm_required=true with the refreshed offer and evaluation_generation; no contract or payment is created. Show the new offer and call Confirm again with the new generation.
//	@Description	When payment_required=true the response includes redirect_url and expires_at only. Direct-activation policies return payment_required=false and status=completed only after authoritative recognition.
//	@Description	Idempotent: repeating Confirm for a session that already linked the same contract replays contract creation and returns the same payment continuation.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			x-auth-subject-id	header		string					true	"Stable authenticated subject identifier"	format(uuid)
//	@Param			x-auth-tenant-id	header		string					true	"Authenticated tenant UUID"					format(uuid)
//	@Param			x-auth-context-id	header		string					true	"Authenticated business-context UUID"		format(uuid)
//	@Param			x-auth-permissions	header		string					true	"Comma-separated gateway permissions"		example(bnpl:checkout-sessions:start:self)
//	@Param			X-Trace-Id			header		string					false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string					false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string					false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			checkout_session_id	path		string					true	"Checkout session UUID"	format(uuid)
//	@Param			body				body		ConfirmRequestSchema	true	"Offer version (evaluation_generation) seen by the customer"
//	@Success		200					{object}	ConfirmSuccessSchema	"Success"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		422					{object}	serdser.ErrorResponse
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
//	@Header			422					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/checkout-sessions/{checkout_session_id}/confirm [post]
func (rs *resource) Confirm(c *gin.Context) {
	input, ok := toConfirmInput(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.Confirm(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	if output.ReconfirmRequired {
		serdser.Ser(
			c,
			http.StatusOK,
			serdser.SuccessCodeCheckoutSessionReconfirmRequired,
			serdser.SuccessMessageCheckoutSessionReconfirmRequired,
			toConfirmResponse(output),
		)

		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutSessionConfirmed,
		serdser.SuccessMessageCheckoutSessionConfirmed,
		toConfirmResponse(output),
	)
}
