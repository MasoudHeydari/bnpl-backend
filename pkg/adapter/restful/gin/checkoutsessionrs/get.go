package checkoutsessionrs

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// Get handles GET /api/v1/checkout-sessions/{checkout_session_id}.
//
//	@Summary		Get hosted checkout session
//	@Description	Read-only projection for the authenticated customer. Never binds a customer, creates a Decision, bumps evaluation_generation, or starts payment. Call POST .../start to prepare or refresh the offer.
//	@Tags			Checkout Sessions
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"					format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"		format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"		example(bnpl:checkout-sessions:read:self)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			checkout_session_id	path		string				true	"Checkout session UUID"	format(uuid)
//	@Success		200					{object}	GetSuccessSchema	"Success"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
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
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/checkout-sessions/{checkout_session_id} [get]
func (rs *resource) Get(c *gin.Context) {
	sessionID, ok := checkoutSessionIDParam(c)
	if !ok {
		return
	}

	identitySubjectID, ok := authenticatedIdentitySubjectID(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.Get(c.Request.Context(), checkoutsessionuc.GetInput{
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
		serdser.SuccessCodeCheckoutSessionRetrieved,
		serdser.SuccessMessageCheckoutSessionRetrieved,
		toGetResponse(output),
	)
}

type getResponse struct {
	CheckoutSessionID    uuid.UUID                   `json:"checkout_session_id" format:"uuid"`
	Status               model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	MerchantOrderID      string                      `json:"merchant_order_id"`
	RequestedAmountMinor uint64                      `json:"requested_amount_minor"`
	Currency             model.Currency              `json:"currency"`
	EvaluationGeneration int                         `json:"evaluation_generation" example:"1"`
	ExpiresAt            time.Time                   `json:"expires_at"`
	Offer                *OfferSchema                `json:"offer,omitempty"`
}

func toGetResponse(output checkoutsessionuc.GetOutput) getResponse {
	resp := getResponse{
		CheckoutSessionID:    output.Session.ID.UUID(),
		Status:               output.Session.Status,
		MerchantOrderID:      output.Session.MerchantOrderID,
		RequestedAmountMinor: output.Session.RequestedAmountMinor,
		Currency:             output.Session.Currency,
		EvaluationGeneration: output.Session.EvaluationGeneration,
		ExpiresAt:            output.Session.ExpiresAt,
	}

	if output.Offer != nil {
		offer := toOfferResponse(*output.Offer)
		resp.Offer = &offer
	}

	return resp
}
