package noop

import (
	"context"
	"fmt"
	"strings"
	"time"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

var _ extcheckoutpayment.Client = (*Client)(nil)

// Client returns a deterministic local checkout payment result without calling Financial Core.
type Client struct{}

// New constructs a noop checkout payment client.
func New() *Client {
	return &Client{}
}

// PrepareCollectionObligation returns a deterministic local obligation without calling Financial Core.
func (c *Client) PrepareCollectionObligation(
	_ context.Context,
	input extcheckoutpayment.PrepareCollectionObligationInput,
) (extcheckoutpayment.PrepareCollectionObligationResult, error) {
	const op = "noop.Client.PrepareCollectionObligation"

	if input.AmountMinor <= 0 || input.Currency == "" || input.SourceID == "" {
		return extcheckoutpayment.PrepareCollectionObligationResult{}, fmt.Errorf("%s: invalid input", op)
	}

	return extcheckoutpayment.PrepareCollectionObligationResult{
		CollectionObligationID: fmt.Sprintf("collection-%s", input.SourceID),
		OutstandingAmountMinor: input.AmountMinor,
		Currency:               input.Currency,
	}, nil
}

// CreateCheckoutPayment returns a local checkout payment session without calling Financial Core.
//
// Local/dev harnesses may still synthesize a payment session. This is not a
// production Financial Core path.
func (c *Client) CreateCheckoutPayment(
	_ context.Context,
	input extcheckoutpayment.CreateCheckoutPaymentInput,
) (extcheckoutpayment.CreateCheckoutPaymentResult, error) {
	const op = "noop.Client.CreateCheckoutPayment"

	if input.AmountMinor == 0 {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf("%s: payment amount is required", op)
	}

	if input.Currency == "" {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf("%s: payment currency is required", op)
	}

	if strings.TrimSpace(input.CollectionObligationID) == "" {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf("%s: collection obligation target is required", op)
	}

	sourceID := input.SourceID
	if sourceID == "" {
		sourceID = "local"
	}

	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(15 * time.Minute)
	}

	return extcheckoutpayment.CreateCheckoutPaymentResult{
		InvoiceID:        fmt.Sprintf("invoice-%s", sourceID),
		PaymentIntentID:  fmt.Sprintf("intent-%s", sourceID),
		PaymentAttemptID: fmt.Sprintf("attempt-%s", sourceID),
		PaymentSessionID: fmt.Sprintf("session-%s", sourceID),
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      input.AmountMinor,
		Currency:         input.Currency,
		PaymentLink:      fmt.Sprintf("https://example.test/payments/%s", sourceID),
		ExpiresAt:        expiresAt,
	}, nil
}

// GetPaymentStatus returns a deterministic non-terminal local status.
func (c *Client) GetPaymentStatus(
	_ context.Context,
	input extcheckoutpayment.GetPaymentStatusInput,
) (extcheckoutpayment.GetPaymentStatusResult, error) {
	return extcheckoutpayment.GetPaymentStatusResult{
		PaymentIntentID: input.PaymentIntentID,
		Status:          extcheckoutpayment.RequiresActionPaymentStatus,
	}, nil
}

// RecordMerchantEntitlement returns a deterministic local entitlement without calling Financial Core.
func (c *Client) RecordMerchantEntitlement(
	_ context.Context,
	input extcheckoutpayment.RecordMerchantEntitlementInput,
) (extcheckoutpayment.RecordMerchantEntitlementResult, error) {
	const op = "noop.Client.RecordMerchantEntitlement"

	if input.SourceID == "" {
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, fmt.Errorf("%s: source id is required", op)
	}

	return extcheckoutpayment.RecordMerchantEntitlementResult{
		MerchantEntitlementID: fmt.Sprintf("entitlement-%s", input.SourceID),
	}, nil
}
