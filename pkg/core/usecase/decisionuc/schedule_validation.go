package decisionuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// Schedule validation failures below are internal consistency checks on
// generated or persisted installment data. They are not client-facing field
// validation, so they use InternalError (not NewPublic) and keep the
// diagnostic fmt.Errorf as cause.

func validateOfferedInstallmentSchedule(
	expectedCount int,
	financedAmountMinor uint64,
	currency model.Currency,
	drafts []checkout.InstallmentDraft,
) error {
	const op = "decisionuc.validateOfferedInstallmentSchedule"

	if expectedCount <= 0 {
		return cerr.InternalError(fmt.Errorf("%s: installment count must be positive", op))
	}

	if err := model.ValidatePositiveAmountMinor(financedAmountMinor); err != nil {
		return cerr.InternalError(fmt.Errorf("%s: financed amount: %w", op, err))
	}

	if !currency.IsValid() {
		return cerr.InternalError(fmt.Errorf("%s: installment currency is invalid", op))
	}

	if len(drafts) != expectedCount {
		return cerr.InternalError(fmt.Errorf("%s: generated installment count does not match expected count", op))
	}

	var principalSum uint64
	seenOrders := make(map[int]struct{}, len(drafts))

	for _, draft := range drafts {
		if draft.Order < 1 || draft.Order > expectedCount {
			return cerr.InternalError(fmt.Errorf("%s: installment order is out of range", op))
		}

		if _, exists := seenOrders[draft.Order]; exists {
			return cerr.InternalError(fmt.Errorf("%s: duplicate installment order", op))
		}

		seenOrders[draft.Order] = struct{}{}

		if err := model.ValidatePositiveAmountMinor(draft.PrincipalAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment principal amount: %w", op, err))
		}

		if err := model.ValidateNonNegativeAmountMinor(draft.UpfrontAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment upfront amount: %w", op, err))
		}

		if err := model.ValidateNonNegativeAmountMinor(draft.BaseFeeAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment base fee amount: %w", op, err))
		}

		if err := model.ValidatePositiveAmountMinor(draft.TotalAmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment total amount: %w", op, err))
		}

		expectedTotal, err := model.SumAmountMinor(
			draft.PrincipalAmountMinor,
			draft.UpfrontAmountMinor,
			draft.BaseFeeAmountMinor,
		)
		if err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment total calculation: %w", op, err))
		}

		if expectedTotal != draft.TotalAmountMinor {
			return cerr.InternalError(fmt.Errorf("%s: installment total amount does not match components", op))
		}

		principalSum, err = model.AddAmountMinor(principalSum, draft.PrincipalAmountMinor)
		if err != nil {
			return cerr.InternalError(fmt.Errorf("%s: installment principal sum: %w", op, err))
		}
	}

	if principalSum != financedAmountMinor {
		return cerr.InternalError(fmt.Errorf("%s: installment principal sum does not match financed amount", op))
	}

	return nil
}

func validatePersistedInstallmentSchedule(
	installments []model.Installment,
	expectedCount int,
	financedAmountMinor uint64,
) error {
	const op = "decisionuc.validatePersistedInstallmentSchedule"

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

func validatePersistedOffer(
	allocation model.CheckoutDecisionAllocation,
	decision model.Decision,
	installments []model.Installment,
	charges []model.InstallmentCharge,
) error {
	const op = "decisionuc.validatePersistedOffer"

	if err := validatePersistedInstallmentSchedule(
		installments,
		allocation.InstallmentCount,
		allocation.ApprovedFinancedAmountMinor,
	); err != nil {
		return err
	}

	installmentIDs := make(map[model.InstallmentID]struct{}, len(installments))
	for _, installment := range installments {
		if installment.DecisionID != decision.ID {
			return cerr.InternalError(fmt.Errorf("%s: persisted installment decision id mismatch", op))
		}

		if installment.CustomerID != allocation.CustomerID {
			return cerr.InternalError(fmt.Errorf("%s: persisted installment customer mismatch", op))
		}

		if installment.MerchantProfileID != allocation.MerchantProfileID {
			return cerr.InternalError(fmt.Errorf("%s: persisted installment merchant mismatch", op))
		}

		if !sameOptionalMerchantBranchProfileID(installment.MerchantBranchProfileID, allocation.MerchantBranchProfileID) {
			return cerr.InternalError(fmt.Errorf("%s: persisted installment merchant branch mismatch", op))
		}

		if installment.Currency != allocation.Currency {
			return cerr.InternalError(fmt.Errorf("%s: persisted installment currency mismatch", op))
		}

		installmentIDs[installment.ID] = struct{}{}
	}

	for _, charge := range charges {
		if _, ok := installmentIDs[charge.InstallmentID]; !ok {
			return cerr.InternalError(fmt.Errorf("%s: persisted charge references an unrelated installment", op))
		}

		if charge.Currency != allocation.Currency {
			return cerr.InternalError(fmt.Errorf("%s: persisted charge currency mismatch", op))
		}

		if err := model.ValidateNonNegativeAmountMinor(charge.AmountMinor); err != nil {
			return cerr.InternalError(fmt.Errorf("%s: persisted charge amount: %w", op, err))
		}

		if charge.ChargeType == model.BNPLFeeInstallmentChargeType &&
			(charge.PolicyID != decision.PolicyID || charge.PolicyVersion != decision.PolicyVersion) {
			return cerr.InternalError(fmt.Errorf("%s: persisted offer charge policy mismatch", op))
		}
	}

	hasFirstInstallment := false
	for _, installment := range installments {
		if installment.InstallmentOrder == 1 {
			hasFirstInstallment = true
			break
		}
	}

	if !hasFirstInstallment {
		return cerr.InternalError(fmt.Errorf("%s: persisted offer is missing first installment", op))
	}

	return nil
}

func sameOptionalMerchantBranchProfileID(a, b *model.MerchantBranchProfileID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return *a == *b
}
