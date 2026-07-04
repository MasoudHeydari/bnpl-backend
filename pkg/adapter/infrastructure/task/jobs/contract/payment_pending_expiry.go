package contract

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// PaymentPendingExpiry returns a task job that expires due payment_pending contracts.
func PaymentPendingExpiry(
	logger corelog.Logger,
	uc *contractuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.ExpireDuePaymentPendingContracts(ctx)
		if err != nil {
			return err
		}

		logger.Info(
			"expired due payment pending contracts",
			"expired_contract_count",
			output.ExpiredContractCount,
			"expired_installment_count",
			output.ExpiredInstallmentCount,
			"canceled_charge_count",
			output.CanceledChargeCount,
		)

		return nil
	}
}
