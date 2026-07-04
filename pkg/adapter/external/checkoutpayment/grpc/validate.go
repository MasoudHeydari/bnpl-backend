package grpc

import (
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

// canonicalizePartyType trims surrounding whitespace and lowercases the value
// so protocol party_type identifiers are stable before validation and
// protobuf serialization. party_id values are never rewritten here.
func canonicalizePartyType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// normalizeCreateCheckoutPaymentInput returns a copy with canonical party_type
// values and validates protocol invariants. The returned input is what must be
// serialized to Financial Core.
func normalizeCreateCheckoutPaymentInput(
	input extcheckoutpayment.CreateCheckoutPaymentInput,
) (extcheckoutpayment.CreateCheckoutPaymentInput, error) {
	normalized := input
	normalized.PayerType = canonicalizePartyType(input.PayerType)
	normalized.PayeeType = canonicalizePartyType(input.PayeeType)

	if err := validateCreateCheckoutPaymentInput(normalized); err != nil {
		return extcheckoutpayment.CreateCheckoutPaymentInput{}, err
	}

	return normalized, nil
}

// validateCreateCheckoutPaymentInput enforces protocol invariants for BNPL
// CreateCheckoutPayment before any Financial Core network call.
//
// Callers must pass already-canonical party_type values.
//
// BNPL purchase party contract:
//   - payer.party_type = customer, payer.party_id nonblank trusted customer id
//   - payee.party_type = platform, payee.party_id exactly empty
func validateCreateCheckoutPaymentInput(input extcheckoutpayment.CreateCheckoutPaymentInput) error {
	const op = "grpc.validateCreateCheckoutPaymentInput"

	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: idempotency key is required", op))
	}

	if strings.TrimSpace(input.SourceType) == "" || strings.TrimSpace(input.SourceID) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: business source reference is required", op))
	}

	switch input.PayerType {
	case extcheckoutpayment.PartyCustomer:
		// ok

	case "subject":
		return cerr.InvalidArgument(fmt.Errorf("%s: legacy payer party_type %q is rejected", op, input.PayerType))

	case extcheckoutpayment.PartyMerchant:
		return cerr.InvalidArgument(fmt.Errorf("%s: payer party_type must be %q", op, extcheckoutpayment.PartyCustomer))

	default:
		return cerr.InvalidArgument(fmt.Errorf("%s: unsupported payer party_type %q", op, input.PayerType))
	}

	if strings.TrimSpace(input.PayerID) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: trusted customer payer id is required", op))
	}

	switch input.PayeeType {
	case extcheckoutpayment.PartyPlatform:
		// Financial Core requires an empty platform party_id for BNPL purchase
		// checkout payments. Do not accept MerchantID or any substitute.
		if input.PayeeID != "" {
			return cerr.InvalidArgument(fmt.Errorf(
				"%s: platform payee id must be empty (got %q)", op, input.PayeeID,
			))
		}

	case extcheckoutpayment.PartyMerchant:
		return cerr.InvalidArgument(fmt.Errorf(
			"%s: payee party_type must be %q (merchant is not the checkout payment payee)",
			op,
			extcheckoutpayment.PartyPlatform,
		))

	default:
		return cerr.InvalidArgument(fmt.Errorf("%s: unsupported payee party_type %q", op, input.PayeeType))
	}

	if strings.TrimSpace(input.CollectionObligationID) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: collection obligation target is required", op))
	}

	if input.AmountMinor <= 0 {
		return cerr.InvalidArgument(fmt.Errorf("%s: payment amount must be positive", op))
	}

	if strings.TrimSpace(input.Currency) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: currency is required", op))
	}

	if strings.TrimSpace(input.Description) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: description is required", op))
	}

	if strings.TrimSpace(input.SuccessReturnURL) == "" ||
		strings.TrimSpace(input.FailureReturnURL) == "" ||
		strings.TrimSpace(input.CancelReturnURL) == "" {
		return cerr.InvalidArgument(fmt.Errorf("%s: return urls are required", op))
	}

	if input.ExpiresAt.IsZero() {
		return cerr.InvalidArgument(fmt.Errorf("%s: expires_at is required", op))
	}

	return nil
}
