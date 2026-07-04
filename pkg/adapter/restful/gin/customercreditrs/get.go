package customercreditrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// GetSuccessSchema is a schema-only success envelope for customer credit summary.
type GetSuccessSchema struct {
	Data creditSummaryResponse           `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// Get handles GET /api/v1/customer-credit.
//
//	@Summary		Get customer credit summary
//	@Description	Returns the authenticated customer's current BNPL credit summary.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.external_customer_id), not the internal BNPL profile primary key.
//	@Description	BNPL resolves or provisions the local customer profile automatically when missing. Profile provisioning is idempotent and does not assign initial credit.
//	@Description	Requires only an authenticated customer subject; no BNPL profile eligibility or credit limit precondition applies.
//	@Description	No request body.
//	@Description	Available credit = effective current credit limit minus active committed financed exposure.
//	@Description	Active financed exposure is persisted on the customer CreditAccount as outstanding principal from activated contracts.
//	@Description	Checkout decisions, installment offers, and pre-activation contracts (payment_pending) do not reduce available credit.
//	@Description	Credit is consumed only when a contract becomes active under its persisted checkout policy.
//	@Description	When no credit limit has been assigned (customer never assessed), data contains only status not_assigned and meta.code is CUSTOMER_CREDIT_NOT_ASSIGNED — clients must initiate credit assessment, not treat this as a zero balance.
//	@Description	When a historical limit exists but is not currently usable (including time-expired), data contains only status expired and expires_at; meta.code is CUSTOMER_CREDIT_EXPIRED — clients must trigger reassessment.
//	@Description	For an active limit, available_amount_minor is an integer minor-unit amount in the response currency.
//	@Tags			Credit Limits
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.external_customer_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																			example(bnpl:credit-limits:read:self)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		200					{object}	GetSuccessSchema	"Customer credit summary returned"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
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
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/customer-credit [get]
func (rs *resource) Get(c *gin.Context) {
	input, ok := toGetInput(c)
	if !ok {
		return
	}

	output, err := rs.customerCredit.GetSummary(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	code := serdser.SuccessCodeCustomerCreditRetrieved
	message := serdser.SuccessMessageCustomerCreditSummaryReturned

	switch output.Status {
	case customercredituc.SummaryStatusExpired:
		code = serdser.SuccessCodeCustomerCreditExpired
		message = serdser.SuccessMessageCustomerCreditExpired

	case customercredituc.SummaryStatusNotAssigned:
		code = serdser.SuccessCodeCustomerCreditNotAssigned
		message = serdser.SuccessMessageCustomerCreditNotAssigned
	}

	serdser.Ser(
		c,
		http.StatusOK,
		code,
		message,
		toCreditSummaryResponse(output),
	)
}
