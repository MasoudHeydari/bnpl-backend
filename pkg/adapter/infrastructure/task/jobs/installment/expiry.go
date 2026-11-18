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

		logger.With(
			"expired_installment_count",
			output.ExpiredInstallmentCount,
			"canceled_charge_count",
			output.CanceledChargeCount,
		).Info(
			corelog.Msg("expired due offered checkout installments"),
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

		logger.With(
			"marked_due_installment_count",
			output.MarkedDueInstallmentCount,
		).Info(
			corelog.Msg("marked scheduled installments due"),
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

		logger.With(
			"marked_overdue_installment_count",
			output.MarkedOverdueInstallmentCount,
		).Info(
			corelog.Msg("marked due installments overdue"),
		)

		return nil
	}
}

// AssessLateFees returns a task job that assesses daily late fees on overdue
// installments (frozen while payment/batch claims are active).
func AssessLateFees(
	logger corelog.Logger,
	uc *installmentlifecycleuc.UseCase,
) task.Job {
	return func(ctx context.Context) error {
		output, err := uc.AssessLateFees(ctx)
		if err != nil {
			return err
		}

		logger.With(
			"assessed_installment_count",
			output.AssessedInstallmentCount,
			"created_charge_count",
			output.CreatedChargeCount,
			"skipped_frozen_count",
			output.SkippedFrozenCount,
		).Info(
			corelog.Msg("assessed overdue installment late fees"),
		)

		return nil
	}
}
