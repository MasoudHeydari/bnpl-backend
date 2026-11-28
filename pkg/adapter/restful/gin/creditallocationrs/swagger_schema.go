package creditallocationrs

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/creditallocationuc"
)

// ListGroupsSuccessSchema documents the ListGroups success envelope.
type ListGroupsSuccessSchema struct {
	Data []model.CreditGroup             `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// PutGroupSuccessSchema documents the PutGroup success envelope.
type PutGroupSuccessSchema struct {
	Data model.CreditGroup               `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// ListEligibilitySuccessSchema documents the ListEligibility success envelope.
type ListEligibilitySuccessSchema struct {
	Data []model.CreditEligibilityEntry  `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// GetEligibilitySuccessSchema documents the GetEligibility success envelope.
type GetEligibilitySuccessSchema struct {
	Data creditallocationuc.EligibilityOutput `json:"data"`
	Meta serdser.SuccessMetaNoPagination      `json:"meta"`
}

// PutEligibilitySuccessSchema documents the PutEligibility success envelope.
type PutEligibilitySuccessSchema struct {
	Data []model.CreditEligibilityEntry  `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// ImportEligibilitySuccessSchema documents the ImportEligibility success envelope.
type ImportEligibilitySuccessSchema struct {
	Data []model.CreditEligibilityEntry  `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// ListRulesSuccessSchema documents the ListRules success envelope.
type ListRulesSuccessSchema struct {
	Data []model.CreditAllocationRule    `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// PutRuleSuccessSchema documents the PutRule success envelope.
type PutRuleSuccessSchema struct {
	Data model.CreditAllocationRule      `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// ListRequestsSuccessSchema documents the ListRequests success envelope.
type ListRequestsSuccessSchema struct {
	Data []creditallocationuc.PendingRequest `json:"data"`
	Meta serdser.SuccessMetaNoPagination     `json:"meta"`
}
