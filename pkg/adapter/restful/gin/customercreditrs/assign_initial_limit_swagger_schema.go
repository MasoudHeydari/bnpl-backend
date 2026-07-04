package customercreditrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// DecisionSchema is a schema-only decision payload.
type DecisionSchema decisionResponse

// CreditLimitSchema is a schema-only credit limit payload.
type CreditLimitSchema creditLimitResponse

// AssignInitialLimitResponseSchema is a schema-only assign-initial-limit response data.
type AssignInitialLimitResponseSchema assignInitialLimitResponse

// AssignInitialLimitSuccessSchema is a schema-only success envelope for initial credit limit assignment.
type AssignInitialLimitSuccessSchema struct {
	Data AssignInitialLimitResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination  `json:"meta"`
}
