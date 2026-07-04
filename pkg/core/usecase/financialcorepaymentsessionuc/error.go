package financialcorepaymentsessionuc

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

func rawPayloadRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_session_payload_required",
		nil,
		cerr.Details{
			Fields: []string{"raw_payload"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.payment_session_payload_required",
					Fields:     []string{"raw_payload"},
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

func bnplPaymentAttemptIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.bnpl_payment_attempt_id_required",
		nil,
		cerr.Details{
			Fields: []string{"bnpl_payment_attempt_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.bnpl_payment_attempt_id_required",
					Fields:     []string{"bnpl_payment_attempt_id"},
				},
			},
		},
	)
}

func financialPaymentIntentIDInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.financial_payment_intent_id_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"financial_payment_intent_id"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.financial_payment_intent_id_required_supported_length",
					Fields:     []string{"financial_payment_intent_id"},
				},
			},
		},
	)
}

func financialPaymentSessionIDInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.financial_payment_session_id_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"financial_payment_session_id"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.financial_payment_session_id_required_supported_length",
					Fields:     []string{"financial_payment_session_id"},
				},
			},
		},
	)
}

func financialPaymentAttemptIDTooLongError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.financial_payment_attempt_id_too_long",
		nil,
		cerr.Details{
			Fields: []string{"financial_payment_attempt_id"},
			Violations: []cerr.Violation{
				{
					Code:       "TOO_LONG",
					MessageKey: "error.financial_payment_attempt_id_too_long",
					Fields:     []string{"financial_payment_attempt_id"},
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

func paymentSessionExpiryInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_session_expiry_invalid",
		nil,
		cerr.Details{
			Fields: []string{"expires_at"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_session_expiry_invalid",
					Fields:     []string{"expires_at"},
				},
			},
		},
	)
}

func paymentURLInvalidError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_url_required_supported_length",
		cause,
		cerr.Details{
			Fields: []string{"payment_url"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_url_required_supported_length",
					Fields:     []string{"payment_url"},
				},
			},
		},
	)
}

func paymentURLMustBeAbsoluteHTTPSError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_url_must_be_an_absolute_https_url_without_user_information",
		cause,
		cerr.Details{
			Fields: []string{"payment_url"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_url_must_be_an_absolute_https_url_without_user_information",
					Fields:     []string{"payment_url"},
				},
			},
		},
	)
}

func paymentSessionExpiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_session_has_already_expired",
		nil,
		cerr.Details{
			Fields: []string{"expires_at"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_session_has_already_expired",
					Fields:     []string{"expires_at"},
				},
			},
		},
	)
}

func paymentAttemptIDMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_attempt_id_does_not_match_event",
		nil,
		cerr.Details{
			Fields: []string{"bnpl_payment_attempt_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_attempt_id_does_not_match_event",
					Fields:     []string{"bnpl_payment_attempt_id"},
				},
			},
		},
	)
}

func paymentAttemptPurposeConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_attempt_purpose_does_not_accept_installment_session_events",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_attempt_purpose_does_not_accept_installment_session_events",
					Fields:     []string{},
				},
			},
		},
	)
}

func paymentAttemptAmountMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_attempt_amount_does_not_match_event",
		nil,
		cerr.Details{
			Fields: []string{"amount_minor"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_attempt_amount_does_not_match_event",
					Fields:     []string{"amount_minor"},
				},
			},
		},
	)
}

func paymentAttemptCurrencyMismatchConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_attempt_currency_does_not_match_event",
		nil,
		cerr.Details{
			Fields: []string{"currency"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.payment_attempt_currency_does_not_match_event",
					Fields:     []string{"currency"},
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
