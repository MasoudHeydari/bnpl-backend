package customercreditrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// AssignInitialLimit handles POST /api/v1/customer-credit/initiate.
//
//	@Summary		Assign all available customer credit products
//	@Description	Assigns the configured initial credit limit after resolving x-auth-mobile-number through Customer Reference Service (ACTIVE only).
//	@Description	CRS customer_id becomes customer_profiles.external_customer_id. The immutable national code links multiple accounts to one credit owner; each account must initiate credit to enroll. Trusted v2 policies are assigned only at the first credit grant to that owner.
//	@Description	The customer profile is created automatically when it does not exist; the profile primary key is assigned by the database.
//	@Description	Requires an authenticated customer subject plus gateway-injected mobile. No request body.
//	@Description	Returns 201 when a new limit is created, 202 for a newly pending request, and 200 for replay or rejected outcomes.
//	@Param			credit_kind	query	string	false	"Optional: select one legacy credit kind; omitted assigns both independent products" Enums(pay_in_installments,pay_next_month)
//	@Tags			Credit Limits
//	@Produce		json
//	@Param			x-auth-subject-id		header		string							true	"Authenticated Identity Service subject UUID"															format(uuid)
//	@Param			x-auth-mobile-number	header		string							true	"Canonical E.164 mobile from JWT auth_mobile_number (gateway-injected)"								example(+989121234567)
//	@Param			x-auth-tenant-id		header		string							true	"Authenticated tenant UUID"																				format(uuid)
//	@Param			x-auth-context-id		header		string							true	"Authenticated business-context UUID"																	format(uuid)
//	@Param			x-auth-permissions		header		string							true	"Comma-separated gateway permissions"																	example(bnpl:credit-limits:create:self)
//	@Param			X-Trace-Id			header		string							false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string							false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string							false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		201					{object}	AssignAllSuccessSchema	"Initial credit limit assigned"
//	@Success		200					{object}	AssignAllSuccessSchema	"Existing request, active credit, or rejected request"
//
// @Success 202 {object} AssignAllSuccessSchema "Credit request received"
// @Header 202 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 202 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 202 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure		400					{object}	serdser.ErrorResponse
//
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		422					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Failure		503					{object}	serdser.ErrorResponse
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
//	@Header			422					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			503					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			503					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			503					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/customer-credit/initiate [post]
func (rs *resource) AssignInitialLimit(c *gin.Context) {
	input, ok := toAssignInitialLimitInput(c)
	if !ok {
		return
	}

	if c.Query("credit_kind") == "" {
		output, err := rs.customerCredit.InitiateAll(c.Request.Context(), input)
		if err != nil {
			serdser.SerErr(c, err)
			return
		}

		assignments := make([]customercredituc.AssignInitialLimitOutput, 0, len(output.Credits))
		for _, item := range output.Credits {
			assignments = append(assignments, item.Assignment)
		}
		status, code, message := assignmentMetadata(assignments)
		serdser.Ser(c, status, code, message, toAllCreditAssignmentsResponse(output))
		return
	}

	output, err := rs.customerCredit.AssignInitialLimit(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	status, code, message := assignmentMetadata([]customercredituc.AssignInitialLimitOutput{output})
	serdser.Ser(c, status, code, message, toAssignInitialLimitResponse(output))
}

// assignmentMetadata separates request acceptance from actual credit grants.
// A mixed result uses the strongest successful outcome; each product retains its own status.
func assignmentMetadata(items []customercredituc.AssignInitialLimitOutput) (int, serdser.SuccessCode, serdser.SuccessMessageKey) {
	pending, created, active, rejected, mismatch, newPending := false, false, false, false, false, false
	for _, item := range items {
		switch item.Result {
		case customercredituc.AssignInitialLimitResultCreated:
			created = true
		case customercredituc.AssignInitialLimitResultAlreadyAssigned:
			active = true
		case customercredituc.AssignInitialLimitResultPending:
			pending = true
			newPending = newPending || item.RequestCreated
		case customercredituc.AssignInitialLimitResultDenied:
			rejected = true
			mismatch = mismatch || (item.CreditRequest != nil && item.CreditRequest.ReasonCode == model.CreditRequestIdentityMismatchReason)
		}
	}
	if created {
		return http.StatusCreated, serdser.SuccessCodeCustomerInitialCreditLimitAssigned, serdser.SuccessMessageInitialCustomerCreditLimitAssigned
	}
	if pending {
		status := http.StatusOK
		if newPending {
			status = http.StatusAccepted
		}
		return status, serdser.SuccessCodeCreditRequestReceived, serdser.SuccessMessageCreditRequestReceived
	}
	if mismatch {
		return http.StatusOK, serdser.SuccessCodeCreditRequestRejected, serdser.SuccessMessageCreditRequestIdentityMismatch
	}
	if rejected && !active {
		return http.StatusOK, serdser.SuccessCodeCreditRequestRejected, serdser.SuccessMessageCreditRequestRejected
	}
	return http.StatusOK, serdser.SuccessCodeCustomerInitialCreditLimitAssigned, serdser.SuccessMessageInitialCustomerCreditLimitAlreadyActive
}
