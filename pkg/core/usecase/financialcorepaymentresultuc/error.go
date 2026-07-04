package financialcorepaymentresultuc

import (
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func eventIDInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_id_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"event_id"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.event_id_required_supported_length",
					Fields:     []string{"event_id"},
				},
			},
		},
	)
}

func sourceServiceInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.source_service_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"source_service"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.source_service_required_supported_length",
					Fields:     []string{"source_service"},
				},
			},
		},
	)
}

func unsupportedSourceServiceError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.source_service_not_supported",
		cause,
		cerr.Details{
			Fields: []string{"source_service"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.source_service_not_supported",
					Fields:     []string{"source_service"},
				},
			},
		},
	)
}

func eventTypeInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_type_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"event_type"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.event_type_required_supported_length",
					Fields:     []string{"event_type"},
				},
			},
		},
	)
}

func unsupportedEventTypeError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_type_not_supported",
		cause,
		cerr.Details{
			Fields: []string{"event_type"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.event_type_not_supported",
					Fields:     []string{"event_type"},
				},
			},
		},
	)
}

func eventVersionInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_version_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"event_version"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.event_version_required_supported_length",
					Fields:     []string{"event_version"},
				},
			},
		},
	)
}

func unsupportedEventVersionError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_version_not_supported",
		cause,
		cerr.Details{
			Fields: []string{"event_version"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.event_version_not_supported",
					Fields:     []string{"event_version"},
				},
			},
		},
	)
}

func rawPayloadRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_result_payload_required",
		nil,
		cerr.Details{
			Fields: []string{"raw_payload"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.payment_result_payload_required",
					Fields:     []string{"raw_payload"},
				},
			},
		},
	)
}

func eventOccurredAtRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_occurred_at_required",
		nil,
		cerr.Details{
			Fields: []string{"event_occurred_at"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.event_occurred_at_required",
					Fields:     []string{"event_occurred_at"},
				},
			},
		},
	)
}

func eventOccurredAtTooFarInFutureError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_occurred_at_is_too_far_in_the_future",
		nil,
		cerr.Details{
			Fields: []string{"event_occurred_at"},
			Violations: []cerr.Violation{
				{
					Code:       "OUT_OF_RANGE",
					MessageKey: "error.event_occurred_at_is_too_far_in_the_future",
					Fields:     []string{"event_occurred_at"},
				},
			},
		},
	)
}

func identifierTooLongError(field string) error {
	params := cerr.MessageParams{"field": humanizeField(field)}
	return cerr.NewPublicWithParams(
		cerr.CodeInvalidArgument,
		"validation.named_field_too_long",
		params,
		nil,
		cerr.Details{
			Fields: []string{field},
			Violations: []cerr.Violation{
				{
					Code:          "TOO_LONG",
					MessageKey:    "validation.named_field_too_long",
					MessageParams: params,
					Fields:        []string{field},
				},
			},
		},
	)
}

func failureCodeTooLongError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.failure_code_too_long",
		nil,
		cerr.Details{
			Fields: []string{"failure_code"},
			Violations: []cerr.Violation{
				{
					Code:       "TOO_LONG",
					MessageKey: "error.failure_code_too_long",
					Fields:     []string{"failure_code"},
				},
			},
		},
	)
}

func amountMinorInvalidError(cause error) error {
	if errors.Is(cause, model.ErrAmountMinorRequired) {
		return cerr.NewPublic(
			cerr.CodeInvalidArgument,
			"error.amount_minor_must_be_greater_than_zero",
			cause,
			cerr.Details{
				Fields: []string{"amount_minor"},
				Violations: []cerr.Violation{
					{
						Code:       "REQUIRED",
						MessageKey: "error.amount_minor_must_be_greater_than_zero",
						Fields:     []string{"amount_minor"},
					},
				},
			},
		)
	}

	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.amount_minor_exceeds_supported_range",
		cause,
		cerr.Details{
			Fields: []string{"amount_minor"},
			Violations: []cerr.Violation{
				{
					Code:       "OUT_OF_RANGE",
					MessageKey: "error.amount_minor_exceeds_supported_range",
					Fields:     []string{"amount_minor"},
				},
			},
		},
	)
}

func paidAtRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.paid_at_required",
		nil,
		cerr.Details{
			Fields: []string{"paid_at"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.paid_at_required",
					Fields:     []string{"paid_at"},
				},
			},
		},
	)
}

func paidAtTooFarInFutureError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.paid_at_is_too_far_in_the_future",
		nil,
		cerr.Details{
			Fields: []string{"paid_at"},
			Violations: []cerr.Violation{
				{
					Code:       "OUT_OF_RANGE",
					MessageKey: "error.paid_at_is_too_far_in_the_future",
					Fields:     []string{"paid_at"},
				},
			},
		},
	)
}

func paidAtAfterEventError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.paid_at_is_after_the_payment_result_event",
		nil,
		cerr.Details{
			Fields: []string{"paid_at"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.paid_at_is_after_the_payment_result_event",
					Fields:     []string{"paid_at"},
				},
			},
		},
	)
}

func currencyInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.currency_invalid",
		nil,
		cerr.Details{
			Fields: []string{"currency"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.currency_invalid",
					Fields:     []string{"currency"},
				},
			},
		},
	)
}

func contractOrSourceRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.contract_id_or_payment_result_source_required",
		nil,
		cerr.Details{
			Fields: []string{"contract_id", "source_type", "source_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.contract_id_or_payment_result_source_required",
					Fields:     []string{"contract_id", "source_type", "source_id"},
				},
			},
		},
	)
}

func paymentIntentIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_intent_id_required",
		nil,
		cerr.Details{
			Fields: []string{"payment_intent_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.payment_intent_id_required",
					Fields:     []string{"payment_intent_id"},
				},
			},
		},
	)
}

func invoiceIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.invoice_id_required",
		nil,
		cerr.Details{
			Fields: []string{"invoice_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.invoice_id_required",
					Fields:     []string{"invoice_id"},
				},
			},
		},
	)
}

func paymentSessionIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_session_id_required",
		nil,
		cerr.Details{
			Fields: []string{"payment_session_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.payment_session_id_required",
					Fields:     []string{"payment_session_id"},
				},
			},
		},
	)
}

func paymentStatusUnsupportedError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_status_not_supported",
		nil,
		cerr.Details{
			Fields: []string{"payment_status"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_status_not_supported",
					Fields:     []string{"payment_status"},
				},
			},
		},
	)
}

func sourceTypeNotContractError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.source_type_does_not_identify_a_bnpl_contract",
		cause,
		cerr.Details{
			Fields: []string{"source_type"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.source_type_does_not_identify_a_bnpl_contract",
					Fields:     []string{"source_type"},
				},
			},
		},
	)
}

func sourceIDInvalidError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.source_id_invalid",
		cause,
		cerr.Details{
			Fields: []string{"source_id"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.source_id_invalid",
					Fields:     []string{"source_id"},
				},
			},
		},
	)
}

func sourceIDNotPaymentAttemptError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.source_id_is_not_a_valid_payment_attempt_id",
		cause,
		cerr.Details{
			Fields: []string{"source_id"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.source_id_is_not_a_valid_payment_attempt_id",
					Fields:     []string{"source_id"},
				},
			},
		},
	)
}

func paymentPurposeMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_purpose_does_not_match_local_payment_attempt",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_purpose_does_not_match_local_payment_attempt",
					Fields:     []string{},
				},
			},
		},
	)
}

func contractIDMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.contract_id_does_not_match_local_payment_attempt",
		nil,
		cerr.Details{
			Fields: []string{"contract_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.contract_id_does_not_match_local_payment_attempt",
					Fields:     []string{"contract_id"},
				},
			},
		},
	)
}

func installmentIDMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_id_does_not_match_local_payment_attempt",
		nil,
		cerr.Details{
			Fields: []string{"installment_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.installment_id_does_not_match_local_payment_attempt",
					Fields:     []string{"installment_id"},
				},
			},
		},
	)
}

func paymentResultSourceMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_result_source_does_not_match_local_payment_attempt",
		nil,
		cerr.Details{
			Fields: []string{"source_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_result_source_does_not_match_local_payment_attempt",
					Fields:     []string{"source_id"},
				},
			},
		},
	)
}

func paymentIntentMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_intent_does_not_match_local_payment_attempt",
		nil,
		cerr.Details{
			Fields: []string{"payment_intent_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_intent_does_not_match_local_payment_attempt",
					Fields:     []string{"payment_intent_id"},
				},
			},
		},
	)
}

func paymentSessionMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_session_does_not_match_local_payment_attempt",
		nil,
		cerr.Details{
			Fields: []string{"payment_session_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_session_does_not_match_local_payment_attempt",
					Fields:     []string{"payment_session_id"},
				},
			},
		},
	)
}

func inboxNotReceivableConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.inbox_event_is_not_receivable",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.inbox_event_is_not_receivable",
					Fields:     []string{},
				},
			},
		},
	)
}

func humanizeField(field string) string {
	switch field {
	case "invoice_id":
		return "Invoice id"

	case "payment_intent_id":
		return "Payment intent id"

	case "payment_attempt_id":
		return "Payment attempt id"

	case "payment_session_id":
		return "Payment session id"

	case "source_type":
		return "Source type"

	case "source_id":
		return "Source id"

	default:
		return field
	}
}
