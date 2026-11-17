package customercreditrs

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// creditSummaryResponse is the customer-facing credit inquiry payload.
//
// Field presence depends on status:
//   - active: limit_amount_minor, available_amount_minor, currency, and expires_at when set
//   - not_assigned: status only (assessment has not occurred)
//   - expired: status and expires_at only (reassessment required)
type creditSummaryResponse struct {
	CreditUsage          customercredituc.CreditUsage `json:"credit_usage"`
	LimitAmountMinor     *uint64                      `json:"limit_amount_minor,omitempty" example:"90000000"`
	AvailableAmountMinor *uint64                      `json:"available_amount_minor,omitempty" example:"4000000"`
	Currency             model.Currency               `json:"currency,omitempty" example:"IRR"`
	// Kept absent on legacy responses; supplied for monthly credit inquiries.
	CreditKind model.CreditKind               `json:"credit_kind,omitempty" enums:"pay_in_4,pay_next_month"`
	Status     customercredituc.SummaryStatus `json:"status" example:"active"`
	ExpiresAt  *time.Time                     `json:"expires_at,omitempty" example:"2027-08-09T12:00:00Z"`
}

func toCreditSummaryResponse(output customercredituc.Output) creditSummaryResponse {
	resp := creditSummaryResponse{
		Status:      output.Status,
		CreditUsage: output.CreditUsage,
	}
	if output.CreditKind == model.PayNextMonthCreditKind {
		resp.CreditKind = output.CreditKind
	}

	if resp.CreditUsage.Status == "" {
		resp.CreditUsage.Status = "unavailable"
		if output.Status == customercredituc.SummaryStatusActive {
			resp.CreditUsage.Status = "allowed"
		}
	}
	switch output.Status {
	case customercredituc.SummaryStatusExpired:
		resp.ExpiresAt = output.ExpiresAt
		return resp

	case customercredituc.SummaryStatusNotAssigned:
		return resp

	default:
		limit := output.LimitAmountMinor
		available := output.AvailableAmountMinor
		resp.LimitAmountMinor = &limit
		resp.AvailableAmountMinor = &available
		resp.Currency = output.Currency
		resp.ExpiresAt = output.ExpiresAt
		return resp
	}
}

// creditApplicationResponse describes the observed assignment lifecycle, not
// a second untracked application state or an FC financial obligation.
type creditApplicationResponse struct {
	Status string `json:"status" enums:"assigned,not_assigned,reassessment_required"`
}

type creditProductResponse struct {
	creditSummaryResponse
	Application creditApplicationResponse `json:"application"`
}

type allCreditSummariesResponse struct {
	Status  customercredituc.AllCreditStatus `json:"status" enums:"not_assigned,partially_assigned,assigned"`
	Credits []creditProductResponse          `json:"credits"`
}

func toAllCreditSummariesResponse(output customercredituc.AllSummariesOutput) allCreditSummariesResponse {
	response := allCreditSummariesResponse{
		Status:  output.Status,
		Credits: make([]creditProductResponse, 0, len(output.Credits)),
	}
	for _, item := range output.Credits {
		summary := toCreditSummaryResponse(item)
		// A uniform product array must identify pay-in-four as well as monthly.
		summary.CreditKind = item.CreditKind
		application := creditApplicationResponse{Status: "not_assigned"}
		switch item.Status {
		case customercredituc.SummaryStatusActive:
			application.Status = "assigned"
		case customercredituc.SummaryStatusExpired:
			application.Status = "reassessment_required"
		}
		response.Credits = append(response.Credits, creditProductResponse{
			creditSummaryResponse: summary,
			Application:           application,
		})
	}
	return response
}

type creditAssignmentResponse struct {
	CreditKind model.CreditKind `json:"credit_kind"`
	assignInitialLimitResponse
}

type allCreditAssignmentsResponse struct {
	Status  customercredituc.AllCreditStatus `json:"status" enums:"not_assigned,partially_assigned,assigned"`
	Credits []creditAssignmentResponse       `json:"credits"`
}

func toAllCreditAssignmentsResponse(output customercredituc.AllAssignmentsOutput) allCreditAssignmentsResponse {
	response := allCreditAssignmentsResponse{
		Status:  output.Status,
		Credits: make([]creditAssignmentResponse, 0, len(output.Credits)),
	}
	for _, item := range output.Credits {
		response.Credits = append(response.Credits, creditAssignmentResponse{
			CreditKind:                 item.CreditKind,
			assignInitialLimitResponse: toAssignInitialLimitResponse(item.Assignment),
		})
	}
	return response
}

func toGetInput(c *gin.Context) (customercredituc.Input, bool) {
	const op = "customercreditrs.toGetInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return customercredituc.Input{}, false
	}

	mobile, ok := authMobileNumber(c, op)
	if !ok {
		return customercredituc.Input{}, false
	}

	// A customer can query each product explicitly, never a combined limit.
	// Unknown products fail at the authenticated HTTP boundary.
	kind := model.CreditKind(c.Query("credit_kind"))
	if !model.EffectiveCreditKind(kind).IsValid() {
		serdser.SerValidationErr(c, "credit_kind", "validation.credit_kind_invalid")
		return customercredituc.Input{}, false
	}
	return customercredituc.Input{
		SubjectID:  authCtx.SubjectID,
		MobileE164: mobile,
		CreditKind: kind,
	}, true
}
