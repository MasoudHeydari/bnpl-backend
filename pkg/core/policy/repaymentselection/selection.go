package repaymentselection

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// requireAllOverdueInstallments enforces the customer's single-payment
// obligation across BOTH credit products and ALL merchants. A scheduled due
// installment is optional until it becomes contractually overdue.
func RequireAllOverdueInstallments(
	selected map[model.InstallmentID]struct{},
	due []model.Installment,
	at time.Time, businessLocation *time.Location,
) error {
	for _, installment := range due {
		if installment.Status == model.OverdueInstallmentStatus ||
			installment.IsOverdueEligible(at, businessLocation) {
			if _, chosen := selected[installment.ID]; !chosen {
				return cerr.Conflict(fmt.Errorf(
					"repayment must include every overdue installment across all credit products",
				))
			}
		}
	}

	return nil
}

// requireConsecutiveInstallments prevents skipping an earlier unpaid
// installment in a four-installment contract. Paid installments do not need
// to be selected again. A monthly contract has one installment and passes.
func RequireConsecutiveInstallments(
	selected map[model.InstallmentID]struct{},
	contractInstallments []model.Installment,
) error {
	// Repository ListByContractID guarantees installment_order ascending.
	unselectedEarlier := false
	for _, installment := range contractInstallments {
		if !installment.Status.IsActive() {
			continue
		}

		_, chosen := selected[installment.ID]
		if chosen && unselectedEarlier {
			return cerr.Conflict(fmt.Errorf(
				"repayment cannot skip an earlier unpaid installment",
			))
		}

		if !chosen {
			unselectedEarlier = true
		}
	}

	return nil
}
