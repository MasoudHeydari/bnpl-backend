package installmentrepaymentuc

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCustomerPayableAmount(t *testing.T) {
	t.Parallel()

	installment := model.Installment{PrincipalAmountMinor: 1_000_000}
	charges := []model.InstallmentCharge{
		{
			AmountMinor:  50_000,
			ChargeStatus: model.AppliedInstallmentChargeStatus,
			PayerType:    model.CustomerInstallmentChargePayerType,
			ChargeType:   model.BNPLFeeInstallmentChargeType,
		},
		{AmountMinor: 10_000, ChargeStatus: model.PaidInstallmentChargeStatus},
	}

	amount, ok := customerPayableAmount(installment, charges)
	assert.True(t, ok)
	assert.Equal(t, uint64(1_050_000), amount)
}

func TestIsPayableInstallmentStatus(t *testing.T) {
	t.Parallel()

	assert.True(t, isPayableInstallmentStatus(model.DueInstallmentStatus))
	assert.False(t, isPayableInstallmentStatus(model.PaidInstallmentStatus))
}

func TestPaymentAttemptRequestFingerprint(t *testing.T) {
	t.Parallel()

	contractID := model.ContractID(uuid.MustParse("00000000-0000-4000-8000-000000000001"))
	installmentID := model.InstallmentID(uuid.MustParse("00000000-0000-4000-8000-000000000002"))
	customerID := model.CustomerID(uuid.MustParse("00000000-0000-4000-8000-000000000003"))

	fp := paymentAttemptRequestFingerprint(
		contractID,
		installmentID,
		customerID,
		model.IRRCurrency,
		model.GatewayPaymentChannel,
	)

	assert.Len(t, fp, 71)
	assert.True(t, strings.HasPrefix(fp, "sha256:"))

	other := paymentAttemptRequestFingerprint(
		contractID,
		installmentID,
		customerID,
		model.IRRCurrency,
		model.WalletPaymentChannel,
	)
	assert.NotEqual(t, fp, other)
}
