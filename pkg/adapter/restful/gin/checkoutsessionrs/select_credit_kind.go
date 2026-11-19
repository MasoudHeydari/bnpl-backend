package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// SelectCreditKind handles PUT /api/v1/partner/checkout-sessions/{checkout_session_id}/credit-kind.
//
//	@Summary		Select or replace partner checkout credit kind
//	@Description	Records or replaces the merchant-site credit-kind selection for an open Partner checkout. Same-kind PUT is idempotent. A different kind on an still-editable open session clears the current Decision linkage and advances evaluation_generation exactly once; prior Decisions remain immutable history. Hosted START cannot override this selection. expected_evaluation_generation is mandatory and must match Partner Create (or the previous Select response) for genuine mutations; same-kind replays may return the current state without advancing generation.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			X-Terminal-Id		header		string							true	"Public numeric terminal identifier of the partner"
//	@Param			X-API-Key			header		string							true	"Secret API key of the partner terminal"
//	@Param			X-Trace-Id			header		string							false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string							false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string							false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			checkout_session_id	path		string							true	"Public checkout session token (cs_<64 hex>)"
//	@Param			body				body		SelectCreditKindRequestSchema	true	"Selected credit kind and expected evaluation generation"
//	@Success		200					{object}	SelectCreditKindSuccessSchema	"Credit kind selected"
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
//	@Router			/api/v1/partner/checkout-sessions/{checkout_session_id}/credit-kind [put]
func (rs *resource) SelectCreditKind(c *gin.Context) {
	token, ok := checkoutSessionTokenParam(c)
	if !ok {
		return
	}

	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return
	}

	var req selectCreditKindRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return
	}

	output, err := rs.checkoutSession.SelectCreditKind(
		c.Request.Context(),
		checkoutsessionuc.SelectCreditKindInput{
			Token:                        token,
			ExternalMerchantID:           merchantID,
			CreditKind:                   req.CreditKind,
			ExpectedEvaluationGeneration: req.ExpectedEvaluationGeneration,
		},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutCreditKindSelected,
		serdser.SuccessMessageCheckoutCreditKindSelected,
		toSelectCreditKindResponse(output),
	)
}
