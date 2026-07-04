package contractuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Schedule validation failures below are internal consistency checks on
// persisted installment data. They are not client-facing field validation, so
// they use InternalError (not NewPublic) and keep the diagnostic fmt.Errorf as
// cause.

func validateInstallmentSchedule(
	installments []model.Installment,
	expectedCount int,
	financedAmountMinor uint64,
) error {
	const op = "contractuc.validateInstallmentSchedule"

	if expectedCount <= 0 {
		return cerr.InternalError(fmt.Errorf("%s: installment count must be positive", op))
	}

	if err := model.ValidatePositiveAmountMinor(financedAmountMinor); err != nil {
		return cerr.InternalError(fmt.Errorf("%s: financed amount: %w", op, err))
	}

	if len(installments) != expectedCount {
		return cerr.InternalError(fmt.Errorf("%s: persisted installment count does not match expected count", op))
	}

	var principalSum uint64
	seenOrders := make(map[int]struct{}, len(installments))

	for _, installment := range installments {
		if installment.InstallmentOrder < 1 || installment.InstallmentOrder > expectedCount {
			return cerr.InternalError(fmt.Errorf("%s: installment order is out of range", op))
		}

		if _, exists := seenOrders[installment.InstallmentOrder]; exists {
			return cerr.InternalError(fmt.Errorf("%s: duplicate installment order", op))
		}

		seenOrders[installment.InstallmentOrder] = struct{}{}

		if err := model.ValidatePositiveAmountMinor(installment.PrincipalAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment principal amount: %w", op, err))
		}

		if err := model.ValidateNonNegativeAmountMinor(installment.UpfrontAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment upfront amount: %w", op, err))
		}

		if err := model.ValidateNonNegativeAmountMinor(installment.BaseFeeAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment base fee amount: %w", op, err))
		}

		if err := model.ValidatePositiveAmountMinor(installment.TotalAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment total amount: %w", op, err))
		}

		if !installment.Currency.IsValid() {
			return cerr.InternalError(fmt.Errorf("%s: installment currency is invalid", op))
		}

		expectedTotal, err := model.SumAmountMinor(
			installment.PrincipalAmountMinor,
			installment.UpfrontAmountMinor,
			installment.BaseFeeAmountMinor,
		)
		if err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment total calculation: %w", op, err))
		}

		if expectedTotal != installment.TotalAmountMinor {
			return cerr.InternalError(fmt.Errorf("%s: installment total amount does not match components", op))
		}

		principalSum, err = model.AddAmountMinor(principalSum, installment.PrincipalAmountMinor)
		if err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment principal sum: %w", op, err))
		}
	}

	if principalSum != financedAmountMinor {
		return cerr.InternalError(fmt.Errorf("%s: installment principal sum does not match financed amount", op))
	}

	return nil
}
