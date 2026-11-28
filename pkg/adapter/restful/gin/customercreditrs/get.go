package customercreditrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// GetSuccessSchema is a schema-only success envelope for customer credit summary.
type GetSuccessSchema struct {
	Data allCreditSummariesResponse      `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// GetSingleSuccessSchema documents the optional legacy credit_kind query.
type GetSingleSuccessSchema struct {
	Data creditSummaryResponse           `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// Get handles GET /api/v1/customer-credit.
//
//	@Summary		Get all independently assigned credit products
//	@Description	Without credit_kind returns pay_in_installments and pay_next_month in one response, even when not assigned.
//	@Description	With credit_kind returns the legacy single-product response for backward compatibility.
//	@Description	Resolves x-auth-mobile-number through Customer Reference Service (ACTIVE only); CRS customer_id is customer_profiles.external_customer_id.
//	@Description	BNPL resolves or provisions the local customer profile automatically when missing. Profile provisioning is idempotent and does not assign initial credit.
//	@Description	Requires an authenticated customer subject plus gateway-injected mobile; no BNPL profile eligibility or credit limit precondition applies.
//	@Description	No request body.
//	@Description	Available credit = effective current credit limit minus active committed financed exposure.
//	@Description	Active financed exposure is persisted on the customer CreditAccount as outstanding principal from activated contracts.
//	@Description	Checkout decisions, installment offers, and pre-activation contracts (payment_pending) do not reduce available credit.
//	@Description	Credit is consumed only when a contract becomes active under its persisted checkout policy.
//	@Description	When no credit limit is assigned, account status is not_assigned; credit_request describes the persisted request if present. Pending requests are not rejected.
//	@Description	When a historical limit is expired, account status is expired; credit_request remains independent.
//	@Description	For an active limit, limit_amount_minor is the governed total capacity and available_amount_minor is remaining usable credit; both are integer minor-unit amounts in the response currency.
//
// @Param credit_kind query string false "Optional single product" Enums(pay_in_installments,pay_next_month)
//
//	@Tags			Credit Limits
//	@Produce		json
//	@Param			x-auth-subject-id		header		string				true	"Authenticated Identity Service subject UUID"															format(uuid)
//	@Param			x-auth-mobile-number	header		string				true	"Canonical E.164 mobile from JWT auth_mobile_number (gateway-injected)"								example(+989121234567)
//	@Param			x-auth-tenant-id		header		string				true	"Authenticated tenant UUID"																				format(uuid)
//	@Param			x-auth-context-id		header		string				true	"Authenticated business-context UUID"																	format(uuid)
//	@Param			x-auth-permissions		header		string				true	"Comma-separated gateway permissions"																	example(bnpl:credit-limits:read:self)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		200					{object}	GetSuccessSchema	"All credit products returned"
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

	if c.Query("credit_kind") == "" {
		output, err := rs.customerCredit.GetAllSummaries(c.Request.Context(), input)
		if err != nil {
			serdser.SerErr(c, err)
			return
		}

		code := serdser.SuccessCodeCustomerCreditRetrieved
		message := serdser.SuccessMessageCustomerCreditSummaryReturned
		if output.Status == customercredituc.AllCreditNotAssigned {
			code = serdser.SuccessCodeCustomerCreditNotAssigned
			message = serdser.SuccessMessageCustomerCreditNotAssigned
		}

		if code == serdser.SuccessCodeCustomerCreditNotAssigned {
			for _, item := range output.Credits {
				if item.CreditRequest != nil {
					code, message = requestMetadata(item.CreditRequest, code, message)
					if code == serdser.SuccessCodeCreditRequestReceived {
						break
					}
				}
			}
		}
		serdser.Ser(c, http.StatusOK, code, message, toAllCreditSummariesResponse(output))
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

	if output.Status != customercredituc.SummaryStatusActive {
		code, message = requestMetadata(output.CreditRequest, code, message)
	}
	serdser.Ser(
		c,
		http.StatusOK,
		code,
		message,
		toCreditSummaryResponse(output),
	)
}

func requestMetadata(r *model.CreditRequestSummary, code serdser.SuccessCode, message serdser.SuccessMessageKey) (serdser.SuccessCode, serdser.SuccessMessageKey) {
	if r != nil {
		switch r.Status {
		case model.PendingCreditRequestStatus, model.ProcessingCreditRequestStatus:
			return serdser.SuccessCodeCreditRequestReceived, serdser.SuccessMessageCreditRequestReceived
		case model.RejectedCreditRequestStatus:
			if r.ReasonCode == model.CreditRequestIdentityMismatchReason {
				return serdser.SuccessCodeCreditRequestRejected, serdser.SuccessMessageCreditRequestIdentityMismatch
			}
			return serdser.SuccessCodeCreditRequestRejected, serdser.SuccessMessageCreditRequestRejected
		}
	}
	return code, message
}
