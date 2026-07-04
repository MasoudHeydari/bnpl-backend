package checkoutsessionrs

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// hostedExternalMerchantResponse is the merchant identity shown on the hosted page.
type hostedExternalMerchantResponse struct {
	DisplayName string `json:"display_name" example:"Example Shop"`
}

// hostedExternalVerificationResponse tells the page how the customer proves the
// checkout phone. destination is masked; the full phone is never returned.
type hostedExternalVerificationResponse struct {
	Type        string `json:"type" enums:"sms_otp" example:"sms_otp"`
	Destination string `json:"destination" example:"0912***4567"`
}

// hostedExternalGetResponse is the read-only hosted page projection.
//
// It includes the prepared offer so the customer can review terms before
// Continue → START. It carries no customer ids, available_credit_minor,
// customer_reference, internal UUID, or full phone.
type hostedExternalGetResponse struct {
	CheckoutSessionID    string                             `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status               model.CheckoutSessionStatus        `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	Merchant             hostedExternalMerchantResponse     `json:"merchant"`
	AmountMinor          uint64                             `json:"amount_minor" example:"120000000"`
	Currency             model.Currency                     `json:"currency" example:"IRR"`
	EvaluationGeneration int                                `json:"evaluation_generation" example:"1"`
	Offer                *hostedExternalOfferSchema         `json:"offer,omitempty"`
	Verification         hostedExternalVerificationResponse `json:"verification"`
	ExpiresAt            time.Time                          `json:"expires_at"`
}

// hostedExternalOfferSchema is the unauthenticated Hosted External offer
// projection. It deliberately omits account-level available_credit_minor:
// the public checkout token is a locator, not authorization to read credit.
type hostedExternalOfferSchema struct {
	Outcome                        checkoutsessionuc.OfferOutcome `json:"outcome" enums:"approved,denied"`
	RequestedAmountMinor           *uint64                        `json:"requested_amount_minor,omitempty"`
	ApprovedFinancedAmountMinor    *uint64                        `json:"approved_financed_amount_minor,omitempty"`
	RequiredDownPaymentAmountMinor *uint64                        `json:"required_down_payment_amount_minor,omitempty"`
	Currency                       model.Currency                 `json:"currency,omitempty"`
	OfferExpiresAt                 *time.Time                     `json:"offer_expires_at,omitempty"`
	InstallmentCount               *int                           `json:"installment_count,omitempty"`
	PayableNowAmountMinor          *uint64                        `json:"payable_now_amount_minor,omitempty"`
	TotalPayableAmountMinor        *uint64                        `json:"total_payable_amount_minor,omitempty"`
	Installments                   []OfferInstallmentSchema       `json:"installments,omitempty"`
	CheckoutAction                 *CheckoutActionSchema          `json:"checkout_action,omitempty"`
}

// hostedExternalStartResponse is the prepared-offer projection after START.
// Offer and evaluation_generation are always returned so deferred (on_start)
// preparation can surface terms before the customer requests OTP.
type hostedExternalStartResponse struct {
	CheckoutSessionID string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"open,unavailable"`
	// EvaluationGeneration versions the offer. Send it back unchanged on OTP and Confirm.
	EvaluationGeneration int `json:"evaluation_generation" example:"1"`
	// Offer is the financing terms bound to evaluation_generation.
	Offer *hostedExternalOfferSchema `json:"offer,omitempty"`
}

// hostedExternalStartRequest carries the offer version the customer saw on GET.
// Omit or send 0 when the offer has not been prepared yet (on_start first START).
type hostedExternalStartRequest struct {
	// EvaluationGeneration is the offer version returned by GET, when known.
	EvaluationGeneration int `json:"evaluation_generation" binding:"omitempty,min=1" example:"1"`
}

// hostedExternalConfirmRequest carries the verification code and the offer
// version the customer saw. The customer, tenant and context are never accepted
// from the client.
type hostedExternalConfirmRequest struct {
	// EvaluationGeneration is the offer version returned by GET / authorized by START.
	EvaluationGeneration int `json:"evaluation_generation" binding:"required,min=1" example:"1"`

	// OTP is the six-digit code delivered by SMS. Format is enforced by the use case.
	OTP string `json:"otp" binding:"required" example:"123456"`
}

// hostedExternalConfirmResponse mirrors confirmResponse but addresses the
// checkout by its public token instead of the internal UUID. Offer uses the
// credit-safe hosted projection (no available_credit_minor).
type hostedExternalConfirmResponse struct {
	CheckoutSessionID    string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status               model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	ReconfirmRequired    bool                        `json:"reconfirm_required" example:"false"`
	EvaluationGeneration int                         `json:"evaluation_generation" example:"2"`
	Offer                *hostedExternalOfferSchema  `json:"offer,omitempty"`
	Contract             *confirmContractResponse    `json:"contract,omitempty"`
	PaymentRequired      bool                        `json:"payment_required" example:"true"`
	RedirectURL          string                      `json:"redirect_url,omitempty" example:"https://pay.example/session/abc"`
	ExpiresAt            string                      `json:"expires_at,omitempty" example:"2026-08-09T12:30:00Z"`
}

func toHostedExternalGetResponse(output checkoutsessionuc.GetExternalOutput) hostedExternalGetResponse {
	resp := hostedExternalGetResponse{
		CheckoutSessionID:    output.Token.String(),
		Status:               output.Status,
		Merchant:             hostedExternalMerchantResponse{DisplayName: output.MerchantDisplayName},
		AmountMinor:          output.AmountMinor,
		Currency:             output.Currency,
		EvaluationGeneration: output.EvaluationGeneration,
		Verification: hostedExternalVerificationResponse{
			Type:        output.Verification.Type,
			Destination: output.Verification.Destination,
		},
		ExpiresAt: output.ExpiresAt,
	}

	if output.Offer != nil {
		safe := toHostedExternalOfferResponse(*output.Offer)
		resp.Offer = &safe
	}

	return resp
}

func toHostedExternalStartResponse(output checkoutsessionuc.StartExternalOutput) hostedExternalStartResponse {
	resp := hostedExternalStartResponse{
		CheckoutSessionID:    output.Session.PublicToken.String(),
		Status:               output.Session.Status,
		EvaluationGeneration: output.EvaluationGeneration,
	}

	if output.Offer.Outcome != "" {
		safe := toHostedExternalOfferResponse(output.Offer)
		resp.Offer = &safe
	}

	return resp
}

func toHostedExternalOfferResponse(offer checkoutsessionuc.Offer) hostedExternalOfferSchema {
	full := toOfferResponse(offer)

	return hostedExternalOfferSchema{
		Outcome:                        full.Outcome,
		RequestedAmountMinor:           full.RequestedAmountMinor,
		ApprovedFinancedAmountMinor:    full.ApprovedFinancedAmountMinor,
		RequiredDownPaymentAmountMinor: full.RequiredDownPaymentAmountMinor,
		Currency:                       full.Currency,
		OfferExpiresAt:                 full.OfferExpiresAt,
		InstallmentCount:               full.InstallmentCount,
		PayableNowAmountMinor:          full.PayableNowAmountMinor,
		TotalPayableAmountMinor:        full.TotalPayableAmountMinor,
		Installments:                   full.Installments,
		CheckoutAction:                 full.CheckoutAction,
	}
}

func toHostedExternalConfirmResponse(output checkoutsessionuc.ConfirmOutput) hostedExternalConfirmResponse {
	base := toConfirmResponse(output)

	var offer *hostedExternalOfferSchema
	if output.Offer.Outcome != "" {
		safe := toHostedExternalOfferResponse(output.Offer)
		offer = &safe
	}

	return hostedExternalConfirmResponse{
		CheckoutSessionID:    output.Session.PublicToken.String(),
		Status:               base.Status,
		ReconfirmRequired:    base.ReconfirmRequired,
		EvaluationGeneration: base.EvaluationGeneration,
		Offer:                offer,
		Contract:             base.Contract,
		PaymentRequired:      base.PaymentRequired,
		RedirectURL:          base.RedirectURL,
		ExpiresAt:            base.ExpiresAt,
	}
}

// GetHostedExternal handles GET /api/v1/checkout-sessions/{token}.
//
//	@Summary		Get hosted external checkout
//	@Description	Unauthenticated read-only hosted page projection. Returns merchant display name, amount, evaluation_generation, the prepared privacy-safe offer (when available), and the masked verification destination. Never creates a Decision, binds a customer, sends OTP, or calls Financial Core.
//	@Tags			Checkout Sessions
//	@Produce		json
//	@Param			token				path		string							true	"Public checkout session token (cs_<64 hex>)"
//	@Success		200					{object}	HostedExternalGetSuccessSchema	"Success"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			200					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Router			/api/v1/checkout-sessions/{token} [get]
func (rs *resource) GetHostedExternal(c *gin.Context) {
	token, ok := checkoutTokenParam(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.GetExternal(
		c.Request.Context(),
		checkoutsessionuc.GetExternalInput{Token: token},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutSessionRetrieved,
		serdser.SuccessMessageCheckoutSessionRetrieved,
		toHostedExternalGetResponse(output),
	)
}

// StartHostedExternal handles POST /api/v1/checkout-sessions/{token}/start.
//
//	@Summary		Start customer checkout (prepare offer if needed)
//	@Description	Unauthenticated token API. Prepares a deferred offer when needed (on_start) or reuses an existing prepared offer. evaluation_generation may be omitted on the first START before an offer exists; the response returns Offer and the authoritative evaluation_generation. Never issues or sends OTP.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			token				path		string								true	"Public checkout session token (cs_<64 hex>)"
//	@Param			body				body		HostedExternalStartRequestSchema	false	"Optional. Omit or send {} on first START before an offer exists; send evaluation_generation when GET already returned one"
//	@Success		200					{object}	HostedExternalStartSuccessSchema	"Success"
//	@Failure		400					{object}	serdser.ErrorResponse
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
//	@Router			/api/v1/checkout-sessions/{token}/start [post]
func (rs *resource) StartHostedExternal(c *gin.Context) {
	token, ok := checkoutTokenParam(c)
	if !ok {
		return
	}

	req, ok := bindHostedExternalStartRequest(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.StartExternal(
		c.Request.Context(),
		checkoutsessionuc.StartExternalInput{
			Token:                token,
			EvaluationGeneration: req.EvaluationGeneration,
		},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutSessionStarted,
		serdser.SuccessMessageCheckoutSessionStarted,
		toHostedExternalStartResponse(output),
	)
}

// bindHostedExternalStartRequest reads an optional JSON body.
//
// An empty body (Content-Length 0) is valid for the first START of a deferred
// (on_start) offer. A non-empty body is bound as JSON; {} leaves generation at 0.
func bindHostedExternalStartRequest(c *gin.Context) (hostedExternalStartRequest, bool) {
	var req hostedExternalStartRequest

	// Empty body selects generation 0 (offer not yet prepared / unknown).
	if c.Request.ContentLength == 0 {
		return req, true
	}

	if !serdser.Bind(c, &req, binding.JSON) {
		return hostedExternalStartRequest{}, false
	}

	return req, true
}

// ConfirmHostedExternal handles POST /api/v1/checkout-sessions/{token}/confirm.
//
//	@Summary		Confirm hosted external checkout
//	@Description	Unauthenticated. Verifies the SMS code, then runs Confirm & Pay for the customer bound to the checkout. A stale evaluation_generation returns reconfirm_required with the refreshed offer.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			token				path		string								true	"Public checkout session token (cs_<64 hex>)"
//	@Param			body				body		HostedExternalConfirmRequestSchema	true	"Offer version and SMS verification code"
//	@Success		200					{object}	HostedExternalConfirmSuccessSchema	"Success"
//	@Failure		400					{object}	serdser.ErrorResponse
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
//	@Router			/api/v1/checkout-sessions/{token}/confirm [post]
func (rs *resource) ConfirmHostedExternal(c *gin.Context) {
	token, ok := checkoutTokenParam(c)
	if !ok {
		return
	}

	var req hostedExternalConfirmRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return
	}

	output, err := rs.checkoutSession.ConfirmExternal(
		c.Request.Context(),
		checkoutsessionuc.ConfirmExternalInput{
			Token:                token,
			OTP:                  strings.TrimSpace(req.OTP),
			EvaluationGeneration: req.EvaluationGeneration,
		},
	)
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
			toHostedExternalConfirmResponse(output),
		)

		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutSessionConfirmed,
		serdser.SuccessMessageCheckoutSessionConfirmed,
		toHostedExternalConfirmResponse(output),
	)
}

// ReturnHostedExternal handles GET /api/v1/checkout-sessions/{token}/return.
//
//	@Summary		Return from hosted external checkout
//	@Description	Unauthenticated browser navigation only. Redirects to the merchant ReturnURL persisted at checkout creation. Never accepts a destination URL from the request. Browser arrival is NOT payment or order-fulfillment authority — merchants must trust Partner Inquiry status=completed or the verified bnpl.checkout.completed.v1 webhook.
//	@Tags			Checkout Sessions
//	@Produce		json
//	@Param			token				path		string	true	"Public checkout session token (cs_<64 hex>)"
//	@Success		302					{string}	string	"Redirect to the pre-registered merchant return URL"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			302					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			302					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			302					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Router			/api/v1/checkout-sessions/{token}/return [get]
func (rs *resource) ReturnHostedExternal(c *gin.Context) {
	token, ok := checkoutTokenParam(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.ReturnExternal(
		c.Request.Context(),
		checkoutsessionuc.ReturnExternalInput{Token: token},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	// Query parameters such as ?url= are intentionally ignored: destination
	// authority is only the persisted merchant ReturnURL.
	c.Redirect(http.StatusFound, output.RedirectURL)
}
