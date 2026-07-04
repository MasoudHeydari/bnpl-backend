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
//   - active: available_amount_minor, currency, and expires_at when set
//   - not_assigned: status only (assessment has not occurred)
//   - expired: status and expires_at only (reassessment required)
type creditSummaryResponse struct {
	AvailableAmountMinor *uint64                        `json:"available_amount_minor,omitempty" example:"4000000"`
	Currency             model.Currency                 `json:"currency,omitempty" example:"IRR"`
	Status               customercredituc.SummaryStatus `json:"status" example:"active"`
	ExpiresAt            *time.Time                     `json:"expires_at,omitempty" example:"2027-08-09T12:00:00Z"`
}

func toCreditSummaryResponse(output customercredituc.Output) creditSummaryResponse {
	resp := creditSummaryResponse{
		Status: output.Status,
	}

	switch output.Status {
	case customercredituc.SummaryStatusExpired:
		resp.ExpiresAt = output.ExpiresAt
		return resp

	case customercredituc.SummaryStatusNotAssigned:
		return resp

	default:
		available := output.AvailableAmountMinor
		resp.AvailableAmountMinor = &available
		resp.Currency = output.Currency
		resp.ExpiresAt = output.ExpiresAt
		return resp
	}
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

	return customercredituc.Input{
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
	}, true
}
