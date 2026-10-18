package contractuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// decisionIDRequiredError returns a public validation error when decision_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
func decisionIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.decision_id_required",
		nil,
		cerr.Details{
			Fields: []string{"decision_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.decision_id_required",
					Fields:     []string{"decision_id"},
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

// providerCodeTooLongError returns a public validation error when provider_code
// exceeds the supported length.
//
// Detected locally from input, so cause is intentionally nil.
func providerCodeTooLongError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.provider_code_too_long",
		nil,
		cerr.Details{
			Fields: []string{"provider_code"},
			Violations: []cerr.Violation{
				{
					Code:       "OUT_OF_RANGE",
					MessageKey: "error.provider_code_too_long",
					Fields:     []string{"provider_code"},
				},
			},
		},
	)
}

//// returnURLRequiredError returns a public validation error when a return URL
//// field is missing. field must be the API/JSON name (e.g. success_return_url).
////
//// Detected locally from input, so cause is intentionally nil.
//func returnURLRequiredError(field string) error {
//	params := cerr.MessageParams{"field": field}
//	return cerr.NewPublicWithParams(
//		cerr.CodeInvalidArgument,
//		"validation.named_field_required",
//		params,
//		nil,
//		cerr.Details{
//			Fields: []string{field},
//			Violations: []cerr.Violation{{
//				Code:          "REQUIRED",
//				MessageKey:    "validation.named_field_required",
//				MessageParams: params,
//				Fields:        []string{field},
//			}},
//		},
//	)
//}
//
//// returnURLTooLongError returns a public validation error when a return URL
//// exceeds the supported length.
////
//// Detected locally from input, so cause is intentionally nil.
//func returnURLTooLongError(field string) error {
//	params := cerr.MessageParams{"field": field}
//	return cerr.NewPublicWithParams(
//		cerr.CodeInvalidArgument,
//		"validation.named_field_too_long",
//		params,
//		nil,
//		cerr.Details{
//			Fields: []string{field},
//			Violations: []cerr.Violation{{
//				Code:          "OUT_OF_RANGE",
//				MessageKey:    "validation.named_field_too_long",
//				MessageParams: params,
//				Fields:        []string{field},
//			}},
//		},
//	)
//}
//
//// returnURLInvalidError returns a public validation error when a return URL is
//// not a valid absolute URI.
////
//// Detected locally from input, so cause is intentionally nil.
//func returnURLInvalidError(field string) error {
//	params := cerr.MessageParams{"field": field}
//	return cerr.NewPublicWithParams(
//		cerr.CodeInvalidArgument,
//		"validation.named_field_invalid",
//		params,
//		nil,
//		cerr.Details{
//			Fields: []string{field},
//			Violations: []cerr.Violation{{
//				Code:          "INVALID",
//				MessageKey:    "validation.named_field_invalid",
//				MessageParams: params,
//				Fields:        []string{field},
//			}},
//		},
//	)
//}
//
//// returnURLSchemeError returns a public validation error when a return URL does
//// not use http or https.
////
//// Detected locally from input, so cause is intentionally nil.
//func returnURLSchemeError(field string) error {
//	params := cerr.MessageParams{"field": field}
//	return cerr.NewPublicWithParams(
//		cerr.CodeInvalidArgument,
//		"validation.named_field_http_scheme",
//		params,
//		nil,
//		cerr.Details{
//			Fields: []string{field},
//			Violations: []cerr.Violation{{
//				Code:          "INVALID",
//				MessageKey:    "validation.named_field_http_scheme",
//				MessageParams: params,
//				Fields:        []string{field},
//			}},
//		},
//	)
//}
//
//// returnURLUserInfoError returns a public validation error when a return URL
//// contains user information.
////
//// Detected locally from input, so cause is intentionally nil.
//func returnURLUserInfoError(field string) error {
//	params := cerr.MessageParams{"field": field}
//	return cerr.NewPublicWithParams(
//		cerr.CodeInvalidArgument,
//		"validation.named_field_no_user_info",
//		params,
//		nil,
//		cerr.Details{
//			Fields: []string{field},
//			Violations: []cerr.Violation{{
//				Code:          "INVALID",
//				MessageKey:    "validation.named_field_no_user_info",
//				MessageParams: params,
//				Fields:        []string{field},
//			}},
//		},
//	)
//}

// checkoutDecisionDoesNotBelongToCustomerError returns a public forbidden error
// when the authenticated customer does not own the checkout decision.
//
// The original cause is preserved for logging/tracing only.
func checkoutDecisionDoesNotBelongToCustomerError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.checkout_decision_does_not_belong_to_customer",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "FORBIDDEN",
					MessageKey: "error.checkout_decision_does_not_belong_to_customer",
					Fields:     []string{},
				},
			},
		},
	)
}

// checkoutDecisionAllocationMissingError returns a public conflict when the
// checkout decision has no allocation available for contract creation.
//
// The original cause is preserved for logging/tracing only.
func checkoutDecisionAllocationMissingError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_decision_allocation_missing",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.checkout_decision_allocation_missing",
					Fields:     []string{},
				},
			},
		},
	)
}

// checkoutDecisionAllocationExpiredError returns a public conflict when the
// checkout decision allocation has expired.
//
// Detected from resource state, so Fields stay empty.
func checkoutDecisionAllocationExpiredError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_decision_allocation_expired",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CHECKOUT_DECISION_ALLOCATION_EXPIRED",
					MessageKey: "error.checkout_decision_allocation_expired",
					Fields:     []string{},
				},
			},
		},
	)
}

// installmentStatusDoesNotAllowContractCreationError returns a public conflict
// when offered installment status does not allow creating a payment-pending
// contract.
//
// Detected from resource state, so Fields stay empty.
func installmentStatusDoesNotAllowContractCreationError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.installment_status_does_not_allow_contract_creation",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.installment_status_does_not_allow_contract_creation",
					Fields:     []string{},
				},
			},
		},
	)
}

// contractAlreadyActiveError returns a public conflict when a contract for the
// decision is already active.
//
// Detected from resource state, so Fields stay empty.
func contractAlreadyActiveError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.contract_is_already_active",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.contract_is_already_active",
					Fields:     []string{},
				},
			},
		},
	)
}

// contractStatusDoesNotAllowCreationError returns a public conflict when an
// existing contract's status blocks creating/reusing a contract for the same
// decision (for example a terminal or unexpected status during idempotent replay).
//
// Detected from resource state, so Fields stay empty.
func contractStatusDoesNotAllowCreationError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.contract_status_does_not_allow_contract_creation",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.contract_status_does_not_allow_contract_creation",
					Fields:     []string{},
				},
			},
		},
	)
}

// checkoutPaymentAttemptIdempotencyConflictError returns a public idempotency
// conflict when a checkout payment attempt does not match the request.
//
// The original cause is preserved for logging/tracing only.
func checkoutPaymentAttemptIdempotencyConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeIdempotencyConflict,
		"error.idempotency_key_conflicts_with_an_existing_request",
		cause,
		cerr.Details{
			Fields: []string{"idempotency_key"},
			Violations: []cerr.Violation{
				{
					Code:       "IDEMPOTENCY_CONFLICT",
					MessageKey: "error.idempotency_key_conflicts_with_an_existing_request",
					Fields:     []string{"idempotency_key"},
				},
			},
		},
	)
}

// contractIDOrPaymentResultSourceRequiredError returns a public validation error
// when neither contract_id nor payment result source identifiers are provided.
//
// Detected locally from input, so cause is intentionally nil.
func contractIDOrPaymentResultSourceRequiredError() error {
	fields := []string{"contract_id", "source_type", "source_id"}
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.contract_id_or_payment_result_source_required",
		nil,
		cerr.Details{
			Fields: fields,
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.contract_id_or_payment_result_source_required",
					Fields:     fields,
				},
			},
		},
	)
}

// invoiceIDRequiredError returns a public validation error when invoice_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
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

// paymentIntentIDRequiredError returns a public validation error when
// payment_intent_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
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

// paymentSessionIDRequiredError returns a public validation error when
// payment_session_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
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

// paidAmountMinorRequiredError returns a public validation error when
// paid_amount_minor is zero.
//
// Detected locally from input, so cause is intentionally nil.
func paidAmountMinorRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.paid_amount_minor_required",
		nil,
		cerr.Details{
			Fields: []string{"paid_amount_minor"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.paid_amount_minor_required",
					Fields:     []string{"paid_amount_minor"},
				},
			},
		},
	)
}

// currencyRequiredError returns a public validation error when currency is missing.
//
// Detected locally from input, so cause is intentionally nil.
func currencyRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.currency_required",
		nil,
		cerr.Details{
			Fields: []string{"currency"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.currency_required",
					Fields:     []string{"currency"},
				},
			},
		},
	)
}

// paidAtRequiredError returns a public validation error when paid_at is missing.
//
// Detected locally from input, so cause is intentionally nil.
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

// eventIDRequiredError returns a public validation error when event_id is missing.
//
// Detected locally from input, so cause is intentionally nil.
func eventIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_id_required",
		nil,
		cerr.Details{
			Fields: []string{"event_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.event_id_required",
					Fields:     []string{"event_id"},
				},
			},
		},
	)
}

// eventOccurredAtRequiredError returns a public validation error when
// event_occurred_at is missing.
//
// Detected locally from input, so cause is intentionally nil.
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

// paymentResultSourceTypeInvalidError returns a public validation error when
// source_type does not identify a BNPL contract.
//
// Detected locally from input, so cause is intentionally nil.
func paymentResultSourceTypeInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_result_source_type_does_not_identify_a_bnpl_contract",
		nil,
		cerr.Details{
			Fields: []string{"source_type"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_result_source_type_does_not_identify_a_bnpl_contract",
					Fields:     []string{"source_type"},
				},
			},
		},
	)
}

// paymentResultSourceIDInvalidError returns a public validation error when
// source_id is not a valid contract UUID.
//
// Detected locally from input, so cause is intentionally nil.
func paymentResultSourceIDInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.payment_result_source_id_invalid",
		nil,
		cerr.Details{
			Fields: []string{"source_id"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.payment_result_source_id_invalid",
					Fields:     []string{"source_id"},
				},
			},
		},
	)
}

// financialCoreCheckoutPaymentUnavailableError returns a public
// service-unavailable error when Financial Core returns an unusable checkout
// payment session response.
//
// cause retains diagnostic detail for logging/tracing and must never be copied
// into Message or Details. Fields stay empty because this is a dependency
// failure, not a client field validation issue.
// financialDelegationUnavailableError fails closed when customer Checkout
// Payment cannot be authorized because Identity financial_delegation is not
// yet integrated. Local PaymentAttempt/Contract state must remain retryable.
func financialDelegationUnavailableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.financial_delegation_is_not_available",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "FORBIDDEN",
					MessageKey: "error.financial_delegation_is_not_available",
					Fields:     []string{},
				},
			},
		},
	)
}

func financialCoreCheckoutPaymentUnavailableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeServiceUnavailable,
		"error.financial_core_checkout_payment_is_temporarily_unavailable",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "SERVICE_UNAVAILABLE",
					MessageKey: "error.financial_core_checkout_payment_is_temporarily_unavailable",
					Fields:     []string{},
				},
			},
		},
	)
}

// checkoutPaymentSessionConflictError returns a public conflict when checkout
// payment session state conflicts with the caller's request or stored session.
//
// Fields stay empty because this is a resource-state / identity conflict.
// cause is preserved for diagnostics only.
func checkoutPaymentSessionConflictError(
	messageKey cerr.MessageKey,
	messageParams cerr.MessageParams,
	cause error,
) error {
	if messageKey == "" {
		messageKey = cerr.DefaultMessageKey(cerr.CodeConflict)
	}

	return cerr.NewPublicWithParams(
		cerr.CodeConflict,
		messageKey,
		messageParams,
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{{
				Code:          "CONFLICT",
				MessageKey:    messageKey,
				MessageParams: messageParams,
				Fields:        []string{},
			}},
		},
	)
}

// checkoutPaymentAttemptTerminalStateError returns a public conflict when the
// checkout payment attempt is already terminal.
func checkoutPaymentAttemptTerminalStateError(state model.PaymentAttemptState) error {
	return checkoutPaymentSessionConflictError(
		"error.checkout_payment_attempt_terminal_state",
		cerr.MessageParams{"state": string(state)},
		nil,
	)
}

// checkoutPaymentAttemptStateCannotAcceptSessionError returns a public conflict
// when the attempt state cannot accept a session response.
func checkoutPaymentAttemptStateCannotAcceptSessionError(state model.PaymentAttemptState) error {
	return checkoutPaymentSessionConflictError(
		"error.checkout_payment_attempt_state_cannot_accept_session",
		cerr.MessageParams{"state": string(state)},
		nil,
	)
}

// contractStateCannotAcceptCheckoutPaymentSessionError returns a public
// conflict when the contract status cannot accept a checkout payment session.
func contractStateCannotAcceptCheckoutPaymentSessionError(status model.ContractStatus) error {
	return checkoutPaymentSessionConflictError(
		"error.contract_state_cannot_accept_checkout_payment_session",
		cerr.MessageParams{"state": string(status)},
		nil,
	)
}

// checkoutPaymentAttemptReconciliationReasonCannotAcceptSessionError returns a
// public conflict when a reconciliation-required attempt cannot be resolved by
// a session response.
func checkoutPaymentAttemptReconciliationReasonCannotAcceptSessionError(
	reason model.PaymentReconciliationReason,
) error {
	return checkoutPaymentSessionConflictError(
		"error.checkout_payment_attempt_reconciliation_reason_cannot_accept_session",
		cerr.MessageParams{"reason": string(reason)},
		nil,
	)
}

// contractIDRequiredError returns a public validation error when contract_id is missing.
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

// contractNotFoundError returns a public not-found error when the contract is missing.
func contractNotFoundError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeResourceNotFound,
		"error.contract_not_found",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_FOUND",
					MessageKey: "error.contract_not_found",
					Fields:     []string{},
				},
			},
		},
	)
}

// contractDoesNotBelongToCustomerError returns a public forbidden error when the
// authenticated customer does not own the contract.
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

// cancellationIdempotencyKeyRequiredError returns a public validation error when
// the cancellation Idempotency-Key is missing or too long.
func cancellationIdempotencyKeyRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.idempotency_key_required_supported_length",
		nil,
		cerr.Details{
			Fields: []string{"idempotency_key"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.idempotency_key_required_supported_length",
					Fields:     []string{"idempotency_key"},
				},
			},
		},
	)
}

// cancellationReasonInvalidError returns a public validation error when reason
// is missing or unsupported.
func cancellationReasonInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.contract_cancellation_reason_invalid",
		nil,
		cerr.Details{
			Fields: []string{"reason"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.contract_cancellation_reason_invalid",
					Fields:     []string{"reason"},
				},
			},
		},
	)
}

// cancellationIdempotencyKeyConflictError returns a public idempotency conflict
// when the same Idempotency-Key is reused with different cancellation inputs.
func cancellationIdempotencyKeyConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeIdempotencyConflict,
		"error.idempotency_key_conflicts_with_an_existing_request",
		cause,
		cerr.Details{
			Fields: []string{"idempotency_key"},
			Violations: []cerr.Violation{
				{
					Code:       "IDEMPOTENCY_CONFLICT",
					MessageKey: "error.idempotency_key_conflicts_with_an_existing_request",
					Fields:     []string{"idempotency_key"},
				},
			},
		},
	)
}

// contractStateDoesNotAllowCancellationError returns a public conflict when the
// contract status cannot enter or continue cancellation through this API.
func contractStateDoesNotAllowCancellationError(status model.ContractStatus) error {
	params := cerr.MessageParams{"state": string(status)}
	return cerr.NewPublicWithParams(
		cerr.CodeConflict,
		"error.contract_state_does_not_allow_cancellation",
		params,
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:          "CONFLICT",
					MessageKey:    "error.contract_state_does_not_allow_cancellation",
					MessageParams: params,
					Fields:        []string{},
				},
			},
		},
	)
}

// contractCancellationAlreadyInProgressError returns a public conflict when a
// different cancellation process is already canceling the contract.
func contractCancellationAlreadyInProgressError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.contract_cancellation_already_in_progress",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.contract_cancellation_already_in_progress",
					Fields:     []string{},
				},
			},
		},
	)
}

// contractCancellationBlockedByPaymentAttemptError returns a public conflict
// while the contract has a non-terminal or financially uncertain payment attempt.
// Cancellation must wait until that attempt reaches an unambiguous terminal state.
func contractCancellationBlockedByPaymentAttemptError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.contract_cancellation_blocked_by_payment_attempt",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.contract_cancellation_blocked_by_payment_attempt",
					Fields:     []string{},
				},
			},
		},
	)
}

// contractCancellationFinancialCoreUnavailableError returns a public
// service-unavailable error when the synchronous cancellation handoff cannot
// be accepted by Financial Core. The durable local canceling intent remains
// available for background retry with the same Financial Core idempotency key.
func contractCancellationFinancialCoreUnavailableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeServiceUnavailable,
		"error.contract_cancellation_financial_core_is_temporarily_unavailable",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "SERVICE_UNAVAILABLE",
					MessageKey: "error.contract_cancellation_financial_core_is_temporarily_unavailable",
					Fields:     []string{},
				},
			},
		},
	)
}

// creditAccountNotUsableForContractError returns a public conflict when the
// customer's credit account cannot accept a new credit-consuming commitment.
func creditAccountNotUsableForContractError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.credit_account_is_not_usable_for_new_contract",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{{
				Code:       "CREDIT_ACCOUNT_NOT_USABLE",
				MessageKey: "error.credit_account_is_not_usable_for_new_contract",
				Fields:     []string{},
			}},
		},
	)
}

// insufficientCreditForContractError returns a public conflict when committed
// exposure leaves less credit capacity than the new contract requires.
func insufficientCreditForContractError(availableAmountMinor, requiredAmountMinor uint64) error {
	params := cerr.MessageParams{
		"available_amount_minor": availableAmountMinor,
		"required_amount_minor":  requiredAmountMinor,
	}
	return cerr.NewPublicWithParams(
		cerr.CodeConflict,
		"error.insufficient_credit_for_contract",
		params,
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{{
				Code:          "INSUFFICIENT_CREDIT",
				MessageKey:    "error.insufficient_credit_for_contract",
				MessageParams: params,
				Fields:        []string{},
			}},
		},
	)
}

// paymentAlreadyInProgressError returns a public conflict when another
// payment_pending contract already occupies this credit account's single
// in-flight payment slot. contractID is retained only in the private cause so
// logs can identify the blocker without leaking internal identifiers through
// the public error contract.
func paymentAlreadyInProgressError(contractID model.ContractID) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.payment_already_in_progress_for_credit_account",
		fmt.Errorf("payment is already in progress for contract %s", contractID),
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{{
				Code:       "PAYMENT_ALREADY_IN_PROGRESS",
				MessageKey: "error.payment_already_in_progress_for_credit_account",
				Fields:     []string{},
			}},
		},
	)
}
