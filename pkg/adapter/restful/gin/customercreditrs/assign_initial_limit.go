package customercreditrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// AssignInitialLimit handles POST /api/v1/customer-credit/initiate.
//
//	@Summary		Assign initial customer credit limit
//	@Description	Assigns the configured initial credit limit to an eligible customer profile identified by x-auth-subject-id.
//	@Description	Subject is the customer identity ID (customer_profiles.identity_subject_id).
//	@Description	The customer profile is created automatically when it does not exist; the profile primary key is assigned by the database.
//	@Description	Requires only an authenticated customer subject. No request body.
//	@Description	Returns 201 when a new limit is created, and 200 for already-assigned or denied outcomes.
//	@Tags			Credit Limits
//	@Produce		json
//	@Param			x-auth-subject-id	header		string							true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.identity_subject_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string							true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string							true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string							true	"Comma-separated gateway permissions"																			example(bnpl:credit-limits:create:self)
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
