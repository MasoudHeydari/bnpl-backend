package creditallocationuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

func invalidInput(field string) error {
	return cerr.NewPublic(cerr.CodeInvalidArgument, "error.credit_allocation_input_invalid", fmt.Errorf("invalid credit allocation field: %s", field), cerr.Details{Fields: []string{field}, Violations: []cerr.Violation{{Code: "INVALID", MessageKey: "error.credit_allocation_input_invalid", Fields: []string{field}}}})
}

func identityConflict() error {
	return cerr.NewPublic(cerr.CodeConflict, "error.credit_eligibility_identity_conflict", nil, cerr.Details{Violations: []cerr.Violation{{Code: "CREDIT_ELIGIBILITY_IDENTITY_CONFLICT", MessageKey: "error.credit_eligibility_identity_conflict"}}})
}

func unsupportedStrategy() error {
	return cerr.NewPublic(cerr.CodeInvalidArgument, "error.credit_allocation_strategy_unavailable", nil, cerr.Details{Fields: []string{"strategy"}, Violations: []cerr.Violation{{Code: "UNAVAILABLE_STRATEGY", MessageKey: "error.credit_allocation_strategy_unavailable", Fields: []string{"strategy"}}}})
}

func unsupportedCurrency() error {
	return cerr.NewPublic(cerr.CodeInvalidArgument, "error.credit_allocation_currency_not_supported", nil, cerr.Details{Fields: []string{"currency"}, Violations: []cerr.Violation{{Code: "CURRENCY_NOT_SUPPORTED", MessageKey: "error.credit_allocation_currency_not_supported", Fields: []string{"currency"}}}})
}
