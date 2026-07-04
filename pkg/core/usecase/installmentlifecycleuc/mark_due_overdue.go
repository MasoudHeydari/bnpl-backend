package installmentlifecycleuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// MarkDueScheduledInstallmentsOutput reports scheduled-to-due transition results.
type MarkDueScheduledInstallmentsOutput struct {
	// MarkedDueInstallmentCount is the number of installments marked due.
	MarkedDueInstallmentCount int64
}

// MarkDueScheduledInstallments transitions scheduled installments to due when due_at has passed.
func (uc *UseCase) MarkDueScheduledInstallments(
	ctx context.Context,
) (MarkDueScheduledInstallmentsOutput, error) {
	now := uc.clock.Now().UTC()
	output := MarkDueScheduledInstallmentsOutput{}

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					installmentQ := uc.installment.Tx(tx)
					outboxQ := uc.outbox.Tx(tx)

					claimed, err := installmentQ.ClaimScheduledDue(
						ctx,
						now,
						uc.MarkDueBatchSize,
					)
					if err != nil {
						return err
					}

					for _, installmentRow := range claimed {
						if !installmentRow.Status.CanTransitionTo(model.DueInstallmentStatus) {
							continue
						}

						updated, err := installmentQ.MarkDue(ctx, installmentRow.ID, now)
						if err != nil {
							if cerr.IsCode(err, cerr.CodeConflict) {
								continue
							}

							return err
						}

						if err = uc.insertLifecycleAuditOutbox(
							ctx,
							outboxQ,
							installmentRow,
							updated,
							now,
						); err != nil {
							return err
						}

						output.MarkedDueInstallmentCount++
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return MarkDueScheduledInstallmentsOutput{}, err
	}

	// Post-commit: count only transitions that survived the transaction.
	for range output.MarkedDueInstallmentCount {
		uc.businessMetrics.InstallmentStateChanged(ctx, string(model.ScheduledInstallmentStatus), string(model.DueInstallmentStatus))
	}

	return output, nil
}

// MarkOverdueDueInstallmentsOutput reports due-to-overdue transition results.
type MarkOverdueDueInstallmentsOutput struct {
	// MarkedOverdueInstallmentCount is the number of installments marked overdue.
	MarkedOverdueInstallmentCount int64
}

// MarkOverdueDueInstallments transitions due installments to overdue when grace has elapsed.
func (uc *UseCase) MarkOverdueDueInstallments(
	ctx context.Context,
) (MarkOverdueDueInstallmentsOutput, error) {
	now := uc.clock.Now().UTC()
	output := MarkOverdueDueInstallmentsOutput{}

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					installmentQ := uc.installment.Tx(tx)
					outboxQ := uc.outbox.Tx(tx)

					claimed, err := installmentQ.ClaimDueOverdueEligible(
						ctx,
						now,
						uc.MarkOverdueBatchSize,
					)
					if err != nil {
						return err
					}

					for _, installmentRow := range claimed {
						if !installmentRow.IsOverdueEligible(now) {
							continue
						}

						if !installmentRow.Status.CanTransitionTo(model.OverdueInstallmentStatus) {
							continue
						}

						updated, err := installmentQ.MarkOverdue(ctx, installmentRow.ID, now)
						if err != nil {
							if cerr.IsCode(err, cerr.CodeConflict) {
								continue
							}

							return err
						}

						if err = uc.insertLifecycleAuditOutbox(
							ctx,
							outboxQ,
							installmentRow,
							updated,
							now,
						); err != nil {
							return err
						}

						output.MarkedOverdueInstallmentCount++
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return MarkOverdueDueInstallmentsOutput{}, err
	}

	// Post-commit: count only transitions that survived the transaction.
	for range output.MarkedOverdueInstallmentCount {
		uc.businessMetrics.InstallmentStateChanged(ctx, string(model.DueInstallmentStatus), string(model.OverdueInstallmentStatus))
	}

	return output, nil
}
