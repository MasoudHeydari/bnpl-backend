package contractuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func firstInstallment(installments []model.Installment) (model.Installment, bool) {
	for _, installment := range installments {
		if installment.InstallmentOrder == 1 {
			return installment, true
		}
	}

	return model.Installment{}, false
}

func installmentIDs(installments []model.Installment) []model.InstallmentID {
	if len(installments) == 0 {
		return nil
	}

	ids := make([]model.InstallmentID, len(installments))
	for i := range installments {
		ids[i] = installments[i].ID
	}

	return ids
}

func installmentChargeIDs(charges []model.InstallmentCharge) []model.InstallmentChargeID {
	if len(charges) == 0 {
		return nil
	}

	ids := make([]model.InstallmentChargeID, len(charges))
	for i := range charges {
		ids[i] = charges[i].ID
	}

	return ids
}

func dedupeInstallmentIDs(ids []model.InstallmentID) []model.InstallmentID {
	if len(ids) == 0 {
		return ids
	}

	seen := make(map[model.InstallmentID]struct{}, len(ids))
	deduped := make([]model.InstallmentID, 0, len(ids))

	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}

	return deduped
}

func dedupeInstallmentChargeIDs(ids []model.InstallmentChargeID) []model.InstallmentChargeID {
	if len(ids) == 0 {
		return ids
	}

	seen := make(map[model.InstallmentChargeID]struct{}, len(ids))
	deduped := make([]model.InstallmentChargeID, 0, len(ids))

	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}

	return deduped
}

func remainingInstallments(installments []model.Installment) []model.Installment {
	remaining := make([]model.Installment, 0, len(installments))

	for _, installment := range installments {
		if installment.InstallmentOrder == 1 {
			continue
		}

		remaining = append(remaining, installment)
	}

	return remaining
}
