package customercreditrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// AssignInitialLimit handles POST /api/v1/customer-credit/initiate.
//
//	@Summary		Assign all available customer credit products
//	@Description	Assigns the configured initial credit limit after resolving x-auth-mobile-number through Customer Reference Service (ACTIVE only).
//	@Description	CRS customer_id becomes customer_profiles.external_customer_id. Trusted-customer policy assignments (v2) key off that same id.
//	@Description	The customer profile is created automatically when it does not exist; the profile primary key is assigned by the database.
//	@Description	Requires an authenticated customer subject plus gateway-injected mobile. No request body.
//	@Description	Returns 201 when a new limit is created, and 200 for already-assigned or denied outcomes.
//	@Param			credit_kind	query	string	false	"Optional: select one legacy credit kind; omitted assigns both independent products" Enums(pay_in_4,pay_next_month)
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
//	@Success		201					{object}	AssignInitialLimitSuccessSchema	"Initial credit limit assigned"
//	@Success		200					{object}	AssignInitialLimitSuccessSchema	"Already assigned or assessment denied"
//	@Failure		400					{object}	serdser.ErrorResponse
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

		status := http.StatusOK
		message := serdser.SuccessMessageInitialCustomerCreditLimitAlreadyActive
		for _, item := range output.Credits {
			if item.Assignment.Result == customercredituc.AssignInitialLimitResultCreated {
				status = http.StatusCreated
				message = serdser.SuccessMessageInitialCustomerCreditLimitAssigned
				break
			}
		}

		serdser.Ser(
			c, status,
			serdser.SuccessCodeCustomerInitialCreditLimitAssigned,
			message, toAllCreditAssignmentsResponse(output),
		)
		return
	}

	output, err := rs.customerCredit.AssignInitialLimit(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	status := http.StatusCreated
	message := serdser.SuccessMessageInitialCustomerCreditLimitAssigned

	switch output.Result {
	case customercredituc.AssignInitialLimitResultAlreadyAssigned:
		status = http.StatusOK
		message = serdser.SuccessMessageInitialCustomerCreditLimitAlreadyActive

	case customercredituc.AssignInitialLimitResultDenied:
		status = http.StatusOK
		message = serdser.SuccessMessageInitialCustomerCreditLimitAssignmentDenied
	}

	serdser.Ser(
		c,
		status,
		serdser.SuccessCodeCustomerInitialCreditLimitAssigned,
		message,
		toAssignInitialLimitResponse(output),
	)
}
