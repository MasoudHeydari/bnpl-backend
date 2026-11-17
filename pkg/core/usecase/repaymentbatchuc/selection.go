package repaymentbatchuc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/repaymentselection"
)

// Keep use-case-local entry points; repository revalidation uses the exact
// same domain policy after acquiring DB locks, eliminating TOCTOU drift.
func requireAllOverdueInstallments(
	selected map[model.InstallmentID]struct{},
	due []model.Installment, at time.Time, loc *time.Location,
) error {
	return repaymentselection.RequireAllOverdueInstallments(selected,due,at,loc)
}

func requireConsecutiveInstallments(
	selected map[model.InstallmentID]struct{},
	contractInstallments []model.Installment,
) error {
	return repaymentselection.RequireConsecutiveInstallments(selected,contractInstallments)
}
