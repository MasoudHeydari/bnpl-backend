package customercreditrs

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

func toAssignInitialLimitInput(c *gin.Context) (customercredituc.AssignInitialLimitInput, bool) {
	const op = "customercreditrs.toAssignInitialLimitInput"

	if _, ok := permission.FromContext(c.Request.Context()); !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return customercredituc.AssignInitialLimitInput{}, false
	}

	mobile, ok := authMobileNumber(c, op)
	if !ok {
		return customercredituc.AssignInitialLimitInput{}, false
	}

	return customercredituc.AssignInitialLimitInput{
		MobileE164: mobile,
	}, true
}

func authMobileNumber(c *gin.Context, op string) (string, bool) {
	values := c.Request.Header.Values(header.AuthMobileNumber)
	if len(values) == 0 {
		serdser.SerErr(c, cerr.NewPublic(
			cerr.CodeInvalidArgument,
			"error.auth_mobile_number_required",
			fmt.Errorf("%s: %s is required", op, header.AuthMobileNumber),
			cerr.Details{
				Fields: []string{header.AuthMobileNumber},
				Violations: []cerr.Violation{
					{
						Code:       "REQUIRED",
						MessageKey: "error.auth_mobile_number_required",
						Fields:     []string{header.AuthMobileNumber},
					},
				},
			},
		))

		return "", false
	}

	if len(values) > 1 {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: %s must appear exactly once", op, header.AuthMobileNumber),
		))

		return "", false
	}

	mobile := strings.TrimSpace(values[0])
	if mobile == "" {
		serdser.SerErr(c, cerr.NewPublic(
			cerr.CodeInvalidArgument,
			"error.auth_mobile_number_required",
			fmt.Errorf("%s: %s is empty", op, header.AuthMobileNumber),
			cerr.Details{
				Fields: []string{header.AuthMobileNumber},
				Violations: []cerr.Violation{
					{
						Code:       "REQUIRED",
						MessageKey: "error.auth_mobile_number_required",
						Fields:     []string{header.AuthMobileNumber},
					},
				},
			},
		))

		return "", false
	}

	return mobile, true
}

type decisionResponse struct {
	Outcome     creditLimitDecisionOutcome `json:"outcome" example:"limit_set" enums:"limit_set,denied"`
	ReasonCodes []string                   `json:"reason_codes"`
}

// creditLimitDecisionOutcome is the assign-initial-limit wire outcome subset.
type creditLimitDecisionOutcome string

const (
	creditLimitOutcomeLimitSet creditLimitDecisionOutcome = "limit_set"
	creditLimitOutcomeDenied   creditLimitDecisionOutcome = "denied"
)

type creditLimitResponse struct {
	AmountMinor   uint64                    `json:"amount_minor" example:"90000000"`
	Currency      model.Currency            `json:"currency" example:"IRR"`
	Status        model.CreditAccountStatus `json:"status" example:"active"`
	EffectiveFrom time.Time                 `json:"effective_from" example:"2026-08-09T12:00:00Z"`
	ExpiresAt     *time.Time                `json:"expires_at,omitempty" example:"2027-08-09T12:00:00Z"`
}

type assignInitialLimitResponse struct {
	Result              customercredituc.AssignInitialLimitResult `json:"result" example:"created"`
	CreditLimitDecision decisionResponse                          `json:"credit_limit_decision"`
	CreditLimit         *creditLimitResponse                      `json:"credit_limit,omitempty"`
}

func toAssignInitialLimitResponse(output customercredituc.AssignInitialLimitOutput) assignInitialLimitResponse {
	decision := output.Decision

	reasonCodes := decision.ReasonCodes
	if reasonCodes == nil {
		reasonCodes = []string{}
	}

	resp := assignInitialLimitResponse{
		Result: output.Result,
		CreditLimitDecision: decisionResponse{
			Outcome:     mapCreditLimitDecisionOutcome(decision.Outcome),
			ReasonCodes: reasonCodes,
		},
	}

	if output.CreditLimit == nil {
		return resp
	}

	limit := output.CreditLimit
	resp.CreditLimit = &creditLimitResponse{
		AmountMinor:   limit.LimitAmountMinor,
		Currency:      limit.Currency,
		Status:        limit.Status,
		EffectiveFrom: limit.EffectiveFrom,
		ExpiresAt:     limit.ExpiresAt,
	}

	return resp
}

func mapCreditLimitDecisionOutcome(outcome model.DecisionOutcome) creditLimitDecisionOutcome {
	switch outcome {
	case model.LimitSetDecisionOutcome:
		return creditLimitOutcomeLimitSet

	case model.DeniedDecisionOutcome:
		return creditLimitOutcomeDenied

	default:
		// Defensive: keep raw wire value rather than inventing approval semantics.
		return creditLimitDecisionOutcome(outcome)
	}
}
