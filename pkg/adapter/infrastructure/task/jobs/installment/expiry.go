package installment

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentlifecycleuc"
)

// Expiry returns a task job that expires offered checkout installments whose
// decision allocation has expired.
func Expiry(
	logger corelog.Logger,
	uc *installmentlifecycleuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.ExpireDueOfferedInstallments(ctx)
		if err != nil {
			return err
		}

		logger.Info(
			"expired due offered checkout installments",
			"expired_installment_count",
			output.ExpiredInstallmentCount,
			"canceled_charge_count",
			output.CanceledChargeCount,
		)

		return nil
	}
}

// MarkDue returns a task job that marks scheduled installments due.
func MarkDue(
	logger corelog.Logger,
	uc *installmentlifecycleuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.MarkDueScheduledInstallments(ctx)
		if err != nil {
			return err
		}

		logger.Info(
			"marked scheduled installments due",
			"marked_due_installment_count", output.MarkedDueInstallmentCount,
		)

		return nil
	}
}

// MarkOverdue returns a task job that marks due installments overdue.
func MarkOverdue(
	logger corelog.Logger,
	uc *installmentlifecycleuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.MarkOverdueDueInstallments(ctx)
		if err != nil {
			return err
		}

		logger.Info(
			"marked due installments overdue",
			"marked_overdue_installment_count", output.MarkedOverdueInstallmentCount,
		)

		return nil
	}
}
