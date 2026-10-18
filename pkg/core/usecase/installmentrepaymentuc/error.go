package installmentrepaymentuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// tenantIDRequiredError returns a public validation error when tenant_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
func tenantIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.tenant_id_required",
		nil,
		cerr.Details{
			Fields: []string{"tenant_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.tenant_id_required",
					Fields:     []string{"tenant_id"},
				},
			},
		},
	)
}

// contextIDRequiredError returns a public validation error when context_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
func contextIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.context_id_required",
		nil,
		cerr.Details{
			Fields: []string{"context_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.context_id_required",
					Fields:     []string{"context_id"},
				},
			},
		},
	)
}

// contractIDRequiredError returns a public validation error when contract_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
func contractIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.contract_id_required",
		nil,
		cerr.Details{
			Fields: []string{"contract_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.contract_id_required",
					Fields:     []string{"contract_id"},
				},
			},
		},
	)
}

// installmentIDRequiredError returns a public validation error when installment_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
func installmentIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.installment_id_required",
		nil,
		cerr.Details{
			Fields: []string{"installment_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.installment_id_required",
					Fields:     []string{"installment_id"},
				},
			},
		},
	)
}

// externalCustomerIDRequiredError returns a public validation error when the
// authenticated Identity Service subject (ExternalCustomerID) is missing from
// use-case input. REST maps this from x-auth-subject-id; clients must not
// supply an internal customer_id.
//
// Detected locally from input, so cause is intentionally nil.
func externalCustomerIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.identity_subject_id_required",
		nil,
		cerr.Details{
			Fields: []string{"identity_subject_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.identity_subject_id_required",
					Fields:     []string{"identity_subject_id"},
				},
			},
		},
	)
}

// idempotencyKeyInvalidError returns a public validation error when
// idempotency_key is missing or exceeds the supported length.
//
// Detected locally from input, so cause is intentionally nil.
func idempotencyKeyInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.idempotency_key_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"idempotency_key"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.idempotency_key_required_supported_length",
					Fields:     []string{"idempotency_key"},
				},
			},
		},
	)
}

// paymentChannelInvalidError returns a public validation error when
// payment_channel is not a supported value.
//
// Detected locally from input, so cause is intentionally nil.
func paymentChannelInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_channel_invalid",
		nil,
		cerr.Details{
			Fields: []string{"payment_channel"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_channel_invalid",
					Fields:     []string{"payment_channel"},
				},
			},
		},
	)
}

//// returnURLInvalidError returns a public validation error when return_url is
//// empty or exceeds the supported length.
////
//// Detected locally from input, so cause is intentionally nil.
//func returnURLInvalidError() error {
//	return cerr.NewPublic(
//		cerr.CodeInvalidArgument,
//		"error.return_url_invalid",
//		nil,
//		cerr.Details{
//			Fields: []string{"return_url"},
//			Violations: []cerr.Violation{
//				{
//					Code:       "INVALID",
//					MessageKey: "error.return_url_invalid",
//					Fields:     []string{"return_url"},
//				},
//			},
//		},
//	)
//}
//
//// returnURLMustBeAbsoluteHTTPError returns a public validation error when
//// return_url is not an absolute http(s) URL without user information.
////
//// Detected locally from input, so cause is intentionally nil.
//func returnURLMustBeAbsoluteHTTPError() error {
//	return cerr.NewPublic(
//		cerr.CodeInvalidArgument,
//		"error.return_url_must_be_an_absolute_http_or_https_url_without_user_information",
//		nil,
//		cerr.Details{
//			Fields: []string{"return_url"},
//			Violations: []cerr.Violation{
//				{
//					Code:       "INVALID",
//					MessageKey: "error.return_url_must_be_an_absolute_http_or_https_url_without_user_information",
//					Fields:     []string{"return_url"},
//				},
//			},
//		},
//	)
//}

// contractDoesNotBelongToCustomerError returns a public forbidden error when the
// authenticated customer does not own the contract (or the customer profile is
// missing and ownership cannot be established).
//
// The original cause is preserved for logging/tracing only.
func contractDoesNotBelongToCustomerError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.contract_does_not_belong_to_customer",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "FORBIDDEN",
					MessageKey: "error.contract_does_not_belong_to_customer",
					Fields:     []string{},
				},
			},
		},
	)
}

// idempotencyKeyConflictError returns a public idempotency conflict when the
// idempotency key was reused with different payment inputs.
//
// The original cause is preserved for logging/tracing only.
func idempotencyKeyConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeIdempotencyConflict,
		"error.idempotency_key_conflicts_with_an_existing_payment_request",
		cause,
		cerr.Details{
			Fields: []string{"idempotency_key"},
			Violations: []cerr.Violation{
				{
					Code:       "IDEMPOTENCY_CONFLICT",
					MessageKey: "error.idempotency_key_conflicts_with_an_existing_payment_request",
					Fields:     []string{"idempotency_key"},
				},
			},
		},
	)
}

// contractNotActiveError returns a public conflict when the contract is not in
// the active state required for installment repayment.
//
// Detected from resource state, so Fields stay empty.
func contractNotActiveError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.contract_is_not_active",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.contract_is_not_active",
					Fields:     []string{},
				},
			},
		},
	)
}

// contractIsCancelingError returns a public conflict when installment repayment
// cannot start because contract cancellation is already in progress.
func contractIsCancelingError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.contract_is_canceling",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.contract_is_canceling",
					Fields:     []string{},
				},
			},
		},
	)
}

// installmentDoesNotBelongToContractError returns a public conflict when the
// installment is not linked to the requested contract.
func installmentDoesNotBelongToContractError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_does_not_belong_to_contract",
		nil,
		cerr.Details{
			Fields: []string{"installment_id", "contract_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.installment_does_not_belong_to_contract",
					Fields:     []string{"installment_id", "contract_id"},
				},
			},
		},
	)
}

// installmentStatusDoesNotAllowPaymentError returns a public conflict when the
// installment status (for example already paid) does not allow repayment.
//
// Detected from resource state, so Fields stay empty.
func installmentStatusDoesNotAllowPaymentError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_status_does_not_allow_payment",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.installment_status_does_not_allow_payment",
					Fields:     []string{},
				},
			},
		},
	)
}

// installmentNotNextPayableError returns a public conflict when the installment
// is not the next payable installment in sequence.
//
// Detected from resource state, so Fields stay empty.
func installmentNotNextPayableError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_is_not_the_next_payable_installment_in_sequence",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.installment_is_not_the_next_payable_installment_in_sequence",
					Fields:     []string{},
				},
			},
		},
	)
}

// installmentPayableAmountZeroError returns a public conflict when there is no
// remaining customer-payable amount on the installment.
func installmentPayableAmountZeroError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_payable_amount_is_zero",
		nil,
		cerr.Details{
			Fields: []string{"installment_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.installment_payable_amount_is_zero",
					Fields:     []string{"installment_id"},
				},
			},
		},
	)
}

// activePaymentAttemptExistsError returns a public conflict when the installment
// already has an active payment attempt.
//
// The original create conflict is preserved for logging/tracing only.
func activePaymentAttemptExistsError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_already_has_an_active_payment_attempt",
		cause,
		cerr.Details{
			Fields: []string{"installment_id"},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.installment_already_has_an_active_payment_attempt",
					Fields:     []string{"installment_id"},
				},
			},
		},
	)
}

// providerCodeTooLongError returns a public validation error when provider_code exceeds the supported length.
func providerCodeTooLongError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.provider_code_too_long",
		nil,
		cerr.Details{
			Fields: []string{"provider_code"},
			Violations: []cerr.Violation{{
				Code:       "INVALID",
				MessageKey: "error.provider_code_too_long",
				Fields:     []string{"provider_code"},
			}},
		},
	)
}

// financialReceivableNotReadyError blocks later repayment until the activation receivable is durably bound.
func financialReceivableNotReadyError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.financial_receivable_not_ready",
		nil,
		cerr.Details{Fields: []string{}, Violations: []cerr.Violation{{
			Code:       "CONFLICT",
			MessageKey: "error.financial_receivable_not_ready",
			Fields:     []string{},
		}}},
	)
}

// installmentChargesRequireFinancialRecognitionError prevents applying unrecognized charges to the principal receivable.
func installmentChargesRequireFinancialRecognitionError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_charges_require_financial_recognition",
		nil,
		cerr.Details{Fields: []string{"installment_id"}, Violations: []cerr.Violation{{
			Code:       "UNSUPPORTED_FINANCIAL_COMPOSITION",
			MessageKey: "error.installment_charges_require_financial_recognition",
			Fields:     []string{"installment_id"},
		}}},
	)
}

// financialCoreInstallmentPaymentUnavailableError reports an invalid or unavailable FC repayment response.
func financialCoreInstallmentPaymentUnavailableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeServiceUnavailable,
		"error.default.service_unavailable",
		cause,
		cerr.Details{Fields: []string{}},
	)
}
