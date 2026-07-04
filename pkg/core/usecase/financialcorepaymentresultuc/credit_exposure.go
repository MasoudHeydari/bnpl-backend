package financialcorepaymentresultuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// lockCreditAccountForPrincipalRelease validates the local credit-capacity
// projection before an authoritative installment success is applied.
func lockCreditAccountForPrincipalRelease(
	ctx context.Context,
	accountQ repo.CreditAccountTxQueryer,
	contractRow model.Contract,
	principalMinor uint64,
) (model.CreditAccount, error) {
	const op = "financialcorepaymentresultuc.lockCreditAccountForPrincipalRelease"
	account, err := accountQ.LockByID(ctx, contractRow.CreditAccountID)
	if err != nil {
		return model.CreditAccount{}, err
	}

	if account.CustomerID != contractRow.CustomerID || account.Currency != contractRow.Currency {
		return model.CreditAccount{}, cerr.InternalError(fmt.Errorf("%s: contract credit-account identity mismatch", op))
	}

	if account.ExposureAmountMinor < principalMinor {
		return model.CreditAccount{}, newReconciliationApplicationError(
			model.ExposureInconsistencyPaymentReconciliationReason,
			fmt.Errorf("%s: credit exposure %d is smaller than principal release %d", op, account.ExposureAmountMinor, principalMinor),
		)
	}

	return account, nil
}

func reduceCreditExposureForInstallment(
	ctx context.Context,
	accountQ repo.CreditAccountTxQueryer,
	account model.CreditAccount,
	installment model.Installment,
	currency model.Currency,
	paidAt time.Time,
	now time.Time,
) error {
	if installment.PrincipalAmountMinor == 0 {
		return nil
	}

	if _, err := accountQ.ReduceExposureIfSufficient(ctx, account.ID, installment.PrincipalAmountMinor, now); err != nil {
		return err
	}

	_, err := accountQ.AppendTransaction(
		ctx,
		account.ID,
		model.ExposureDecreasedCreditTransactionType,
		installment.PrincipalAmountMinor,
		currency,
		model.InstallmentPaidCreditTransactionReason,
		model.InstallmentCreditTransactionReferenceType,
		installment.ID.UUID(),
		paidAt.UTC(),
		now,
	)
	return err
}
