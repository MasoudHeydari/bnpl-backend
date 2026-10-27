package collectionsuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func newCreditUsageRestrictedError() error {
	params := cerr.MessageParams{
		"reason_code": model.CollectionsDefaultBlocksNewCreditDecisionReason,
	}

	return cerr.NewPublicWithParams(
		cerr.CodeConflict,
		"error.collections_new_credit_usage_restricted",
		params,
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:          "CONFLICT",
					MessageKey:    "error.collections_new_credit_usage_restricted",
					MessageParams: params,
					Fields:        []string{},
				},
			},
		},
	)
}
