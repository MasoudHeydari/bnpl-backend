package installmentrepaymentuc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func customerPayableAmount(
	installment model.Installment,
	charges []model.InstallmentCharge,
) (uint64, bool) {
	amount, ok, err := model.CustomerPayableAmount(installment, charges)
	if err != nil || !ok {
		return 0, false
	}

	return amount, true
}

type paymentAttemptFingerprintContext struct {
	TenantID     string
	ContextID    string
	ProviderCode string
}

func paymentAttemptRequestFingerprint(
	contractID model.ContractID,
	installmentID model.InstallmentID,
	customerID model.CustomerID,
	currency model.Currency,
	channel model.PaymentChannel,
	contexts ...paymentAttemptFingerprintContext,
) string {
	var context paymentAttemptFingerprintContext
	if len(contexts) > 0 {
		context = contexts[0]
	}

	raw := fmt.Sprintf(
		"contract:%s:installment:%s:customer:%s:currency:%s:channel:%s:tenant:%s:context:%s:provider:%s",
		contractID,
		installmentID,
		customerID,
		currency,
		channel,
		strings.TrimSpace(context.TenantID),
		strings.TrimSpace(context.ContextID),
		strings.TrimSpace(context.ProviderCode),
	)

	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func installmentBelongsToContract(
	installment model.Installment,
	contractID model.ContractID,
) bool {
	return installment.ContractID != nil && *installment.ContractID == contractID
}

func isPayableInstallmentStatus(status model.InstallmentStatus) bool {
	switch status {
	case model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus:
		return true

	default:
		return false
	}
}
