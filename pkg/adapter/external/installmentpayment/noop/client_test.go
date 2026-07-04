package noop

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/external/installmentpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCreateInstallmentPayment(t *testing.T) {
	t.Parallel()

	clk := mock.NewClock(t)
	now := time.Date(
		2026,
		8,
		31,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	clk.EXPECT().Now().Return(now)

	client, err := New(clk)
	require.NoError(t, err)
	returnURL := "https://example.com/return"

	result, err := client.CreateInstallmentPayment(
		context.Background(),
		installmentpayment.CreateInstallmentPaymentInput{
			ContractID:     model.ContractID(uuid.New()),
			InstallmentID:  model.InstallmentID(uuid.New()),
			CustomerID:     model.CustomerID(uuid.New()),
			AmountMinor:    25_000_000,
			Currency:       string(model.IRRCurrency),
			Channel:        model.GatewayPaymentChannel,
			IdempotencyKey: "key-1",
			ReturnURL:      &returnURL,
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, result.FinancialPaymentIntentID)
	require.NotEmpty(t, result.FinancialPaymentSessionID)
	require.NotNil(t, result.PaymentURL)
	require.NotNil(t, result.ExpiresAt)
}
