package decisionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// Create handles POST /api/v1/checkout-decisions.
//
//	@Summary		Create checkout decision
//	@Description	Evaluates checkout feasibility for a customer/merchant pair and persists a decision.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.external_customer_id). Do not send customer_id in the body.
//	@Description	purchase_context.type must be "pos" or "online"; the channel is explicit and is never inferred from optional fields.
//	@Description	For type=pos, purchase_context.pos_gps_id is required; online_store_id and merchant_order_id are forbidden.
//	@Description	For type=online, purchase_context.online_store_id is required; pos_gps_id is forbidden; merchant_order_id is optional (max 256 characters).
//	@Description	Malformed purchase_context combinations are rejected with HTTP 400 before the use case runs.
//	@Description	BNPL resolves the selling point and merchant from the supplied source identifier; do not send merchant_profile_id or branch_id.
//	@Description	Requires an authenticated customer subject, an eligible customer profile, and an active credit limit.
//	@Description	An approved (including domain conditional / partial financing) checkout Decision atomically creates and returns the persisted installment offer
//	@Description	(installment IDs, status, order, due date, principal, upfront, fees, totals, payable-now, and total payable).
//	@Description	For policies anchored to contract_activation, offered due dates are provisional; installment economics stay fixed and due dates are finalized from the successful activation instant.
//	@Description	checkout_action tells the client how to confirm the offer: confirm_and_pay requires the returned initial payment; confirm_purchase creates the contract without an initial payment.
//	@Description	Frontend clients should drive the checkout button from checkout_action, not from policy version or purchase channel.
//	@Description	The HTTP response outcome is always "approved" or "denied"; persisted conditional outcomes are mapped to "approved".
//	@Description	Denied decisions return reason codes and omit installment_offer; no installment records are created.
//	@Description	Approval means the customer had enough available credit at evaluation time (limit minus active exposure).
//	@Description	Does not reserve or reduce available credit; final capacity is checked again at contract activation.
//	@Description	Idempotent via Idempotency-Key (checkout_intent_id / checkout key). Returns 201 on create and 200 on idempotent replay
//	@Description	with the same Decision and persisted installments (no duplicates).
//	@Tags			Decisions
//	@Accept			json
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.external_customer_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																			example(bnpl:decisions:create:self)
//	@Param			Idempotency-Key		header		string				true	"Client idempotency token used as the checkout key (max 256 characters)"
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			body				body		CreateRequestSchema	true	"Checkout decision request"
//	@Success		201					{object}	CreateSuccessSchema	"Decision created"
//	@Success		200					{object}	CreateSuccessSchema	"Idempotent replay of an existing decision"
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
//	@Router			/api/v1/checkout-decisions [post]
func (rs *resource) Create(c *gin.Context) {
	input, ok := toCreateInput(c)
	if !ok {
		return
	}

	output, err := rs.decision.Create(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	status := http.StatusCreated
	message := serdser.SuccessMessageCheckoutDecisionCreated

	if output.Result == decisionuc.CreateResultAlreadyExists {
		status = http.StatusOK
		message = serdser.SuccessMessageCheckoutDecisionAlreadyExists
	}

	serdser.Ser(
		c,
		status,
		serdser.SuccessCodeDecisionCreated,
		message,
		toCreateResponse(output),
	)
}
