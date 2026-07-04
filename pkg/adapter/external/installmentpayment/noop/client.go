package noop

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extinstallmentpayment "github.com/parstis/bnpl-backend/pkg/core/external/installmentpayment"
)

// Client implements the installment payment client for local development.
type Client struct {
	clock clock.Clock
}

var _ extinstallmentpayment.Client = (*Client)(nil)

// New constructs a noop installment payment client.
func New(clk clock.Clock) (*Client, error) {
	const op = "noop.New"

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	return &Client{clock: clk}, nil
}

// CreateInstallmentPayment returns a deterministic local Financial Core session.
func (c *Client) CreateInstallmentPayment(
	_ context.Context,
	input extinstallmentpayment.CreateInstallmentPaymentInput,
) (extinstallmentpayment.CreateInstallmentPaymentResult, error) {
	expiresAt := c.clock.Now().UTC().Add(15 * time.Minute)
	paymentURL := fmt.Sprintf(
		"https://financial-core.local/pay/installment/%s",
		input.InstallmentID,
	)

	return extinstallmentpayment.CreateInstallmentPaymentResult{
		FinancialPaymentIntentID:  fmt.Sprintf("intent-%s", input.InstallmentID),
		FinancialPaymentSessionID: fmt.Sprintf("session-%s", input.InstallmentID),
		PaymentURL:                &paymentURL,
		ExpiresAt:                 &expiresAt,
		ReusedIdempotentResult:    false,
	}, nil
}
