package decisionuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// invalidCheckoutKeyError translates a model.NormalizeCheckoutKey failure into
// a client-safe public error.
//
// The normalize error is preserved as cause for diagnostics; its raw text must
// never become a public localized message.
func invalidCheckoutKeyError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.checkout_key_invalid",
		cause,
		cerr.Details{
			Fields: []string{"checkout_key"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.checkout_key_invalid",
					Fields:     []string{"checkout_key"},
				},
			},
		},
	)
}

// checkoutKeyRequiredError returns a public validation error when checkout_key
// is missing from the use case input.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func checkoutKeyRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.checkout_key_required",
		nil,
		cerr.Details{
			Fields: []string{"checkout_key"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.checkout_key_required",
					Fields:     []string{"checkout_key"},
				},
			},
		},
	)
}

// identitySubjectIDRequiredError returns a public validation error when the
// authenticated Identity Service subject (IdentitySubjectID) is missing from
// use-case input. REST maps this from x-auth-subject-id; clients must not
// supply an internal customer_id.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func identitySubjectIDRequiredError() error {
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

// posGpsIDRequiredError returns a public validation error when purchase_context.pos_gps_id is
// missing for a POS checkout.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func posGpsIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.pos_gps_id_required",
		nil,
		cerr.Details{
			Fields: []string{"purchase_context.pos_gps_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.pos_gps_id_required",
					Fields:     []string{"purchase_context.pos_gps_id"},
				},
			},
		},
	)
}

// purchaseChannelInvalidError returns a public validation error when the
// purchase channel is missing or unsupported.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func purchaseChannelInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.purchase_channel_invalid",
		nil,
		cerr.Details{
			Fields: []string{"purchase_context.type"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.purchase_channel_invalid",
					Fields:     []string{"purchase_context.type"},
				},
			},
		},
	)
}

// onlineStoreIDRequiredError returns a public validation error when an online
// checkout has no store identity.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func onlineStoreIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.online_store_id_required",
		nil,
		cerr.Details{
			Fields: []string{"purchase_context.online_store_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.online_store_id_required",
					Fields:     []string{"purchase_context.online_store_id"},
				},
			},
		},
	)
}

// posGpsIDNotAllowedError rejects POS data on an online purchase context.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func posGpsIDNotAllowedError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.pos_gps_id_not_allowed_for_online_purchase",
		nil,
		cerr.Details{
			Fields: []string{"purchase_context.pos_gps_id"},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_ALLOWED",
					MessageKey: "error.pos_gps_id_not_allowed_for_online_purchase",
					Fields:     []string{"purchase_context.pos_gps_id"},
				},
			},
		},
	)
}

// onlineStoreIDNotAllowedError rejects online-store data on a physical POS
// purchase context.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func onlineStoreIDNotAllowedError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.online_store_id_not_allowed_for_pos_purchase",
		nil,
		cerr.Details{
			Fields: []string{"purchase_context.online_store_id"},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_ALLOWED",
					MessageKey: "error.online_store_id_not_allowed_for_pos_purchase",
					Fields:     []string{"purchase_context.online_store_id"},
				},
			},
		},
	)
}

// merchantOrderIDNotAllowedError rejects merchant order references on physical
// POS purchases.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func merchantOrderIDNotAllowedError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.merchant_order_id_not_allowed_for_pos_purchase",
		nil,
		cerr.Details{
			Fields: []string{"purchase_context.merchant_order_id"},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_ALLOWED",
					MessageKey: "error.merchant_order_id_not_allowed_for_pos_purchase",
					Fields:     []string{"purchase_context.merchant_order_id"},
				},
			},
		},
	)
}

// merchantOrderIDTooLongError rejects merchant order references that exceed
// persisted bounds.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func merchantOrderIDTooLongError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.merchant_order_id_too_long",
		nil,
		cerr.Details{
			Fields: []string{"purchase_context.merchant_order_id"},
			Violations: []cerr.Violation{
				{
					Code:       "MAX_LENGTH",
					MessageKey: "error.merchant_order_id_too_long",
					Fields:     []string{"purchase_context.merchant_order_id"},
				},
			},
		},
	)
}

// requestedAmountRequiredError returns a public validation error when
// requested_amount_minor is zero.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func requestedAmountRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.requested_amount_must_be_greater_than_zero",
		nil,
		cerr.Details{
			Fields: []string{"requested_amount_minor"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.requested_amount_must_be_greater_than_zero",
					Fields:     []string{"requested_amount_minor"},
				},
			},
		},
	)
}

// requestedAmountOutOfRangeError returns a public validation error when
// requested_amount_minor exceeds the supported storage/financial-core range.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func requestedAmountOutOfRangeError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.requested_amount_exceeds_supported_range",
		nil,
		cerr.Details{
			Fields: []string{"requested_amount_minor"},
			Violations: []cerr.Violation{
				{
					Code:       "OUT_OF_RANGE",
					MessageKey: "error.requested_amount_exceeds_supported_range",
					Fields:     []string{"requested_amount_minor"},
				},
			},
		},
	)
}

// currencyInvalidError returns a public validation error when currency is not
// a supported value.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
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

// idempotencyConflictError translates a reused idempotency key with different
// checkout inputs into the public IDEMPOTENCY_CONFLICT contract.
//
// The diagnostic cause is preserved for logging; Fields stays empty because
// this is a request-level / state conflict rather than a single-field
// validation failure. Callers may also attribute it to idempotency_key.
func idempotencyConflictError(cause error) error {
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

// checkoutDecisionCreateUnresolvedConflictError is returned when concurrent
// create raced and the use case could not reconcile to a stable decision.
//
// This is a state-level conflict with no single invalid input field; Fields
// is intentionally empty. There is no lower-level technical cause to wrap.
func checkoutDecisionCreateUnresolvedConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_decision_could_not_be_created_due_to_a_concurrent_conflict",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.checkout_decision_could_not_be_created_due_to_a_concurrent_conflict",
					Fields:     []string{},
				},
			},
		},
	)
}
