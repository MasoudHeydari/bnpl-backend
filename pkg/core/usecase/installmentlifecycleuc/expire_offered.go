package installmentlifecycleuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ExpireDueOfferedInstallmentsOutput reports expiry maintenance results for
// unattached offered installments.
type ExpireDueOfferedInstallmentsOutput struct {
	// ExpiredInstallmentCount is the number of offered installments expired.
	ExpiredInstallmentCount int64
	// CanceledChargeCount is the number of installment charges canceled.
	CanceledChargeCount int64
}

// ExpireDueOfferedInstallments expires unattached offered installments whose
// checkout decision allocation has expired, and cancels their offered charges.
// Credit reservations are not involved; capacity is claimed only at activation.
func (uc *UseCase) ExpireDueOfferedInstallments(
	ctx context.Context,
) (ExpireDueOfferedInstallmentsOutput, error) {
	now := uc.clock.Now().UTC()
	output := ExpireDueOfferedInstallmentsOutput{}

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					installmentQ := uc.installment.Tx(tx)
					installmentChargeQ := uc.installmentCharge.Tx(tx)
					outboxQ := uc.outbox.Tx(tx)

					canceledCharges, err := installmentChargeQ.CancelDueOfferedByAllocationExpiry(
						ctx,
						now,
						now,
						uc.MarkDueBatchSize,
					)
					if err != nil {
						return err
					}

					expiredInstallments, err := installmentQ.ExpireDueOfferedByAllocationExpiry(
						ctx,
						now,
						now,
						uc.MarkDueBatchSize,
					)
					if err != nil {
						return err
					}

					chargesByInstallmentID := groupCanceledChargesByInstallmentID(canceledCharges)
					for _, installmentRow := range expiredInstallments {
						if err := uc.insertOfferExpiryAuditOutbox(
							ctx,
							outboxQ,
							installmentRow,
							chargesByInstallmentID[installmentRow.ID],
							now,
						); err != nil {
							return err
						}
					}

					output.CanceledChargeCount = int64(len(canceledCharges))
					output.ExpiredInstallmentCount = int64(len(expiredInstallments))

					return nil
				},
			)
		},
	)
	if err != nil {
		return ExpireDueOfferedInstallmentsOutput{}, err
	}

	return output, nil
}

func groupCanceledChargesByInstallmentID(
	charges []model.InstallmentCharge,
) map[model.InstallmentID][]model.InstallmentCharge {
	grouped := make(map[model.InstallmentID][]model.InstallmentCharge, len(charges))
	for _, charge := range charges {
		grouped[charge.InstallmentID] = append(grouped[charge.InstallmentID], charge)
	}

	return grouped
}
