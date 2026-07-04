package contractuc

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ActivateFromFinancialCorePaymentResult activates a payment_pending contract after
// Financial Core reports a final successful payment result.
//
// Inbound payment-result consumers should prefer financialcorepaymentresultuc.Process
// for durable inbox handling.
func (uc *UseCase) ActivateFromFinancialCorePaymentResult(
	ctx context.Context,
	input ActivateFromFinancialCorePaymentResultInput,
) (ActivateOutput, error) {
	if err := validateActivateFromFinancialCorePaymentResultInput(input); err != nil {
		return ActivateOutput{}, err
	}

	contractID, err := resolveContractID(input)
	if err != nil {
		return ActivateOutput{}, err
	}

	now := uc.clock.Now().UTC()
	var output ActivateOutput

	err = uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					contractQ := uc.contract.Tx(tx)
					contractRow, lockErr := contractQ.LockByID(ctx, contractID)
					if lockErr != nil {
						return lockErr
					}

					if contractRow.Status == model.ExpiredContractStatus {
						return validateExpiredContractPayment(contractRow, input.PaidAt)
					}

					var activateErr error
					output, activateErr = uc.activateFromFinancialCorePaymentResultInTx(
						ctx,
						tx,
						input,
						contractRow,
						now,
					)

					return activateErr
				},
			)
		},
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	return output, nil
}

// ApplyCheckoutPaymentActivationInTx activates a payment_pending contract inside an existing transaction.
func (uc *UseCase) ApplyCheckoutPaymentActivationInTx(
	ctx context.Context,
	tx repo.Tx,
	input ActivateFromFinancialCorePaymentResultInput,
	contractRow model.Contract,
	now time.Time,
) (ActivateOutput, error) {
	return uc.activateFromFinancialCorePaymentResultInTx(
		ctx,
		tx,
		input,
		contractRow,
		now,
	)
}

func (uc *UseCase) activateFromFinancialCorePaymentResultInTx(
	ctx context.Context,
	tx repo.Tx,
	input ActivateFromFinancialCorePaymentResultInput,
	contractRow model.Contract,
	now time.Time,
) (ActivateOutput, error) {
	const op = "contractuc.UseCase.activateFromFinancialCorePaymentResultInTx"

	contractQ := uc.contract.Tx(tx)
	allocationQ := uc.checkoutDecisionAllocation.Tx(tx)
	installmentQ := uc.installment.Tx(tx)
	installmentChargeQ := uc.installmentCharge.Tx(tx)
	creditAccountQ := uc.creditAccount.Tx(tx)

	paymentAttemptQ := uc.paymentAttempt.Tx(tx)
	checkoutAttempt, attemptErr := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx,
		contractRow.ID,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if attemptErr != nil {
		if cerr.IsCode(attemptErr, cerr.CodeResourceNotFound) {
			return ActivateOutput{}, newReconciliationError(
				model.UnknownPaymentAttemptPaymentReconciliationReason,
				fmt.Errorf("%s: checkout payment attempt is missing", op),
			)
		}

		return ActivateOutput{}, attemptErr
	}

	if isContractStateCompatibleWithAppliedCheckoutPayment(contractRow.Status) {
		return uc.resolveAlreadyAppliedCheckout(
			ctx,
			contractQ,
			installmentQ,
			contractRow,
			input,
			now,
		)
	}

	if contractRow.Status != model.PaymentPendingContractStatus {
		return ActivateOutput{}, newReconciliationError(
			model.InvalidContractStatePaymentReconciliationReason,
			fmt.Errorf("%s: contract status does not allow activation", op),
		)
	}

	if err := validateCheckoutPaymentAttemptIdentity(contractRow, checkoutAttempt, input); err != nil {
		return ActivateOutput{}, err
	}

	if err := validatePaymentOccurredInTime(contractRow, input.PaidAt); err != nil {
		return ActivateOutput{}, err
	}

	if input.Currency != contractRow.Currency {
		return ActivateOutput{}, newReconciliationError(
			model.PaymentCurrencyMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: payment currency does not match contract currency", op),
		)
	}

	if input.PaidAmountMinor != contractRow.PayableNowAmountMinor {
		return ActivateOutput{}, newReconciliationError(
			model.PaymentAmountMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: paid amount does not match payable now amount", op),
		)
	}

	if err := contractRow.Status.ValidateTransitionTo(model.ActiveContractStatus); err != nil {
		return ActivateOutput{}, cerr.InternalError(err)
	}

	allocation, err := allocationQ.LockByDecisionID(ctx, contractRow.DecisionID)
	if err != nil {
		return ActivateOutput{}, err
	}

	if err = validateAllocationPaidInTime(allocation, input.PaidAt); err != nil {
		return ActivateOutput{}, err
	}

	installments, err := installmentQ.LockByDecisionID(ctx, contractRow.DecisionID)
	if err != nil {
		return ActivateOutput{}, err
	}

	first, ok := firstInstallment(installments)
	if !ok {
		return ActivateOutput{}, newReconciliationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: first installment is missing", op),
		)
	}

	if err = validateInstallmentsForActivation(contractRow, installments, first); err != nil {
		return ActivateOutput{}, err
	}

	// Lock, validate, and classify the exact first-installment charge rows before any
	// mutation so only customer-payable debit charges are settled and all monetary
	// invariants hold against one exact payable amount.
	settlement, err := lockClassifyFirstInstallmentCharges(
		ctx,
		installmentChargeQ,
		contractRow,
		first,
		checkoutAttempt.Items,
		input,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	remainingPrincipal := contractRow.ApprovedFinancedAmountMinor - first.PrincipalAmountMinor

	// The capacity decision was made before the payment session was opened.
	// Do not re-evaluate the current limit after Financial Core reports success:
	// the customer has already paid. We only lock the account here so exposure
	// and its immutable journal entry are committed atomically with activation.
	creditAccount, err := creditAccountQ.LockByID(ctx, contractRow.CreditAccountID)
	if err != nil {
		return ActivateOutput{}, err
	}

	if creditAccount.Currency != contractRow.Currency {
		return ActivateOutput{}, cerr.InternalError(fmt.Errorf("%s: contract credit account currency mismatch", op))
	}

	if err = first.Status.ValidateTransitionTo(model.PaidInstallmentStatus); err != nil {
		return ActivateOutput{}, cerr.InternalError(err)
	}

	for _, installment := range remainingInstallments(installments) {
		nextStatus := model.ScheduledInstallmentStatus
		if !installment.DueAt.After(now) {
			nextStatus = model.DueInstallmentStatus
		}

		if err = installment.Status.ValidateTransitionTo(nextStatus); err != nil {
			return ActivateOutput{}, cerr.InternalError(err)
		}
	}

	boundContract, err := contractQ.BindSuccessfulCheckoutPaymentIdentity(
		ctx,
		contractRow.ID,
		input.InvoiceID,
		input.PaymentIntentID,
		input.PaymentSessionID,
		normalizeOptionalProviderAttemptID(input.FinancialPaymentAttemptID),
		now,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	contractRow = boundContract

	paymentReference := input.PaymentIntentID
	paymentReferencePtr := &paymentReference

	activatedContract, err := contractQ.MarkActive(
		ctx,
		contractRow.ID,
		paymentReferencePtr,
		&checkoutAttempt.ID,
		normalizeOptionalProviderAttemptID(input.FinancialPaymentAttemptID),
		remainingPrincipal,
		input.PaidAt,
		now,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	firstPaid, err := installmentQ.MarkPaid(
		ctx,
		first.ID,
		contractRow.ID,
		first.TotalAmountMinor,
		input.PaidAt,
		now,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	updatedRemaining := make([]model.Installment, 0, len(installments)-1)
	for _, installment := range remainingInstallments(installments) {
		nextStatus := model.ScheduledInstallmentStatus
		if !installment.DueAt.After(now) {
			nextStatus = model.DueInstallmentStatus
		}

		updated, markErr := installmentQ.MarkScheduledOrDue(
			ctx,
			installment.ID,
			contractRow.ID,
			nextStatus,
			now,
		)
		if markErr != nil {
			return ActivateOutput{}, markErr
		}

		updatedRemaining = append(updatedRemaining, updated)
	}

	paidChargeCount, err := installmentChargeQ.MarkSnapshottedChargeIDsPaid(
		ctx,
		settlement.PaidChargeIDs,
		input.PaidAt,
		now,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	if paidChargeCount != int64(len(settlement.PaidChargeIDs)) {
		return ActivateOutput{}, cerr.InternalError(fmt.Errorf(
			"%s: unexpected first installment paid charge count %d, expected %d", op, paidChargeCount,
			len(settlement.PaidChargeIDs),
		))
	}

	// First-installment non-debit charges (customer credits, merchant/platform charges)
	// transition to applied so no valid charge is left offered after the installment is paid.
	firstAppliedCount, err := installmentChargeQ.MarkChargeIDsApplied(
		ctx,
		settlement.AppliedChargeIDs,
		now,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	if firstAppliedCount != int64(len(settlement.AppliedChargeIDs)) {
		return ActivateOutput{}, cerr.InternalError(fmt.Errorf(
			"%s: unexpected first installment applied charge count %d, expected %d", op, firstAppliedCount,
			len(settlement.AppliedChargeIDs),
		))
	}

	remainingIDs := installmentIDs(updatedRemaining)
	appliedChargeCount, err := installmentChargeQ.MarkOfferedChargesAppliedByInstallmentIDs(
		ctx,
		remainingIDs,
		now,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	appliedChargeCount += firstAppliedCount

	if remainingPrincipal > 0 {
		if _, err = creditAccountQ.AddExposure(ctx, creditAccount.ID, remainingPrincipal, now); err != nil {
			return ActivateOutput{}, err
		}

		if _, err = creditAccountQ.AppendTransaction(
			ctx,
			creditAccount.ID,
			model.ExposureIncreasedCreditTransactionType,
			remainingPrincipal,
			contractRow.Currency,
			model.ContractActivatedCreditTransactionReason,
			model.ContractCreditTransactionReferenceType,
			contractRow.ID.UUID(),
			input.PaidAt,
			now,
		); err != nil {
			return ActivateOutput{}, err
		}
	}

	return ActivateOutput{
		Result:                         ActivateResultActivated,
		Contract:                       activatedContract,
		FirstInstallment:               firstPaid,
		RemainingInstallments:          updatedRemaining,
		PaidChargeCount:                paidChargeCount,
		AppliedChargeCount:             appliedChargeCount,
		AddedActiveExposureAmountMinor: remainingPrincipal,
	}, nil
}

// isContractStateCompatibleWithAppliedCheckoutPayment reports whether a contract state can
// back an already-applied checkout first-installment payment acknowledgement.
func isContractStateCompatibleWithAppliedCheckoutPayment(status model.ContractStatus) bool {
	switch status {
	case model.ActiveContractStatus,
		model.CancelingContractStatus,
		model.CanceledContractStatus,
		model.CompletedContractStatus:
		return true

	default:
		return false
	}
}

// resolveAlreadyAppliedCheckout handles a duplicate successful checkout payment for a contract
// already in an applied-compatible state. It proves the first installment is paid and validates
// the payment against immutable persisted application evidence, then safely enriches missing
// final provider-attempt audit metadata before returning an already-applied result.
//
// It deliberately does not read or recompute from current charge rows: charges are mutable
// after activation, so a later fee, discount, or waiver must not reclassify the original
// applied payment.
func (uc *UseCase) resolveAlreadyAppliedCheckout(
	ctx context.Context,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	contractRow model.Contract,
	input ActivateFromFinancialCorePaymentResultInput,
	now time.Time,
) (ActivateOutput, error) {
	const op = "contractuc.UseCase.resolveAlreadyAppliedCheckout"

	if err := validateCheckoutAlreadyAppliedIdentity(contractRow, input); err != nil {
		return ActivateOutput{}, err
	}

	// Lock installment rows for the decision so the first installment paid-state proof uses
	// a locked row rather than a non-locking read.
	installments, lockErr := installmentQ.LockByDecisionID(ctx, contractRow.DecisionID)
	if lockErr != nil {
		return ActivateOutput{}, lockErr
	}

	first, ok := firstInstallment(installments)
	if !ok {
		return ActivateOutput{}, newReconciliationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: first installment is missing", op),
		)
	}

	if first.ContractID == nil || *first.ContractID != contractRow.ID {
		return ActivateOutput{}, newReconciliationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: first installment does not belong to the contract", op),
		)
	}

	if first.Status != model.PaidInstallmentStatus {
		return ActivateOutput{}, newReconciliationError(
			model.InvalidInstallmentStatePaymentReconciliationReason,
			fmt.Errorf("%s: first installment is not paid for an active checkout contract", op),
		)
	}

	if err := validateCheckoutPaymentAlreadyApplied(contractRow, first, input); err != nil {
		return ActivateOutput{}, err
	}

	enrichedContract, err := uc.enrichSuccessfulProviderAttemptID(
		ctx,
		contractQ,
		contractRow,
		input,
		now,
	)
	if err != nil {
		return ActivateOutput{}, err
	}

	return ActivateOutput{
		Result:                         ActivateResultAlreadyActive,
		Contract:                       enrichedContract,
		FirstInstallment:               first,
		RemainingInstallments:          remainingInstallments(installments),
		AddedActiveExposureAmountMinor: 0,
	}, nil
}

// enrichSuccessfulProviderAttemptID fills in the final successful provider-attempt id when it
// is currently absent and the incoming payment supplies one. Conflicting non-empty values are
// rejected by validateCheckoutAlreadyAppliedIdentity before this point, so enrichment only
// occurs when the stored value is NULL.
func (uc *UseCase) enrichSuccessfulProviderAttemptID(
	ctx context.Context,
	contractQ repo.ContractTxQueryer,
	contractRow model.Contract,
	input ActivateFromFinancialCorePaymentResultInput,
	now time.Time,
) (model.Contract, error) {
	incoming := normalizeOptionalProviderAttemptID(input.FinancialPaymentAttemptID)
	if incoming == nil {
		return contractRow, nil
	}

	if contractRow.SuccessfulFinancialPaymentAttemptID != nil {
		return contractRow, nil
	}

	return contractQ.EnrichSuccessfulFinancialPaymentAttemptID(ctx, contractRow.ID, *incoming, now)
}

// validateCheckoutPaymentAlreadyApplied proves a duplicate successful checkout payment matches
// the original application using only immutable persisted evidence.
//
// contract.PayableNowAmountMinor, first.TotalAmountMinor and first.PaidAmountMinor are frozen
// after checkout activation (none of them are rewritten by any later write path), so they are
// authoritative historical evidence of what was paid. Current charge rows are mutable and are
// intentionally never consulted here, so a fee, discount, waiver, merchant charge, or platform
// charge added after activation cannot turn a valid duplicate into reconciliation_required.
func validateCheckoutPaymentAlreadyApplied(
	contractRow model.Contract,
	first model.Installment,
	input ActivateFromFinancialCorePaymentResultInput,
) error {
	const op = "contractuc.validateCheckoutPaymentAlreadyApplied"

	if input.Currency != contractRow.Currency || input.Currency != first.Currency {
		return newReconciliationError(
			model.PaymentCurrencyMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: payment currency does not match contract and first installment currency", op),
		)
	}

	if first.PaidAt == nil {
		return newReconciliationError(
			model.InvalidInstallmentStatePaymentReconciliationReason,
			fmt.Errorf("%s: first installment has no recorded paid_at for an applied checkout payment", op),
		)
	}

	if input.PaidAmountMinor != contractRow.PayableNowAmountMinor ||
		input.PaidAmountMinor != first.TotalAmountMinor ||
		input.PaidAmountMinor != first.PaidAmountMinor {
		return newReconciliationError(
			model.PaymentAmountMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: paid amount does not match the applied checkout payment evidence", op),
		)
	}

	return nil
}

// firstInstallmentChargeTransitions holds the exact charge IDs that must transition during
// first-installment checkout activation after pre-mutation validation succeeds.
type firstInstallmentChargeTransitions struct {
	PaidChargeIDs    []model.InstallmentChargeID
	AppliedChargeIDs []model.InstallmentChargeID
}

// lockClassifyFirstInstallmentCharges locks the exact charge rows snapshotted for the first
// installment, validates them, classifies their settlement lifecycle by payer/effect/status,
// and asserts the full first-installment monetary equality before any mutation. It returns a
// ReconciliationError when the charge snapshot cannot back an exact first-installment payment.
func lockClassifyFirstInstallmentCharges(
	ctx context.Context,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	contractRow model.Contract,
	first model.Installment,
	items []model.PaymentAttemptItem,
	input ActivateFromFinancialCorePaymentResultInput,
) (firstInstallmentChargeTransitions, error) {
	const op = "contractuc.lockClassifyFirstInstallmentCharges"

	charges, err := installmentChargeQ.ListByInstallmentIDs(ctx, []model.InstallmentID{first.ID})
	if err != nil {
		return firstInstallmentChargeTransitions{}, err
	}

	locked, err := installmentChargeQ.LockByIDs(ctx, installmentChargeIDs(charges))
	if err != nil {
		return firstInstallmentChargeTransitions{}, err
	}

	if err := validateCheckoutPaymentAttemptSnapshot(first, locked, items); err != nil {
		return firstInstallmentChargeTransitions{}, err
	}

	transitions, classifyErr := classifyFirstInstallmentChargesForActivation(first, contractRow.Currency, locked)
	if classifyErr != nil {
		return firstInstallmentChargeTransitions{}, classifyErr
	}

	payable, ok, err := model.CustomerPayableAmount(first, locked)
	if err != nil {
		return firstInstallmentChargeTransitions{}, newReconciliationError(
			model.InvalidInstallmentStatePaymentReconciliationReason,
			fmt.Errorf("%s: first installment payable amount is invalid: %w", op, err),
		)
	}

	if !ok {
		return firstInstallmentChargeTransitions{}, newReconciliationError(
			model.PaymentAmountMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: first installment payable amount could not be computed", op),
		)
	}

	if err = validateFirstInstallmentMonetaryEquality(contractRow, first, payable, input); err != nil {
		return firstInstallmentChargeTransitions{}, err
	}

	return transitions, nil
}

func validateCheckoutPaymentAttemptSnapshot(
	first model.Installment,
	charges []model.InstallmentCharge,
	items []model.PaymentAttemptItem,
) error {
	const op = "contractuc.validateCheckoutPaymentAttemptSnapshot"

	if len(items) == 0 {
		return newReconciliationError(
			model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
			fmt.Errorf("%s: checkout payment attempt snapshot is missing", op),
		)
	}

	payable, ok := model.PayableAmountFromItems(items)
	if !ok || payable != first.TotalAmountMinor {
		return newReconciliationError(
			model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
			fmt.Errorf("%s: checkout payment attempt snapshot total is inconsistent", op),
		)
	}

	principalCount := 0
	upfrontCount := 0
	snapshotCharges := make(map[uuid.UUID]model.PaymentAttemptItem)
	for _, item := range items {
		switch item.ItemType {
		case model.PrincipalPaymentAttemptItemType:
			principalCount++
			if item.SourceID != first.ID.UUID() ||
				item.AmountMinor != first.PrincipalAmountMinor ||
				item.Effect != model.DebitPaymentAttemptItemEffect {
				return newReconciliationError(
					model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
					fmt.Errorf("%s: checkout payment attempt principal snapshot is inconsistent", op),
				)
			}

		case model.UpfrontPaymentAttemptItemType:
			upfrontCount++
			if item.SourceID != first.ID.UUID() ||
				item.AmountMinor != first.UpfrontAmountMinor ||
				item.Effect != model.DebitPaymentAttemptItemEffect {
				return newReconciliationError(
					model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
					fmt.Errorf("%s: checkout payment attempt upfront snapshot is inconsistent", op),
				)
			}

		case model.ChargePaymentAttemptItemType:
			if item.SourceID == uuid.Nil {
				return newReconciliationError(
					model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
					fmt.Errorf("%s: checkout payment attempt charge snapshot has an empty source", op),
				)
			}

			if _, exists := snapshotCharges[item.SourceID]; exists {
				return newReconciliationError(
					model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
					fmt.Errorf("%s: checkout payment attempt charge snapshot is duplicated", op),
				)
			}

			snapshotCharges[item.SourceID] = item

		default:
			return newReconciliationError(
				model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
				fmt.Errorf("%s: checkout payment attempt snapshot contains an unsupported item type", op),
			)
		}
	}

	if principalCount != 1 {
		return newReconciliationError(
			model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
			fmt.Errorf("%s: checkout payment attempt snapshot must contain exactly one principal item", op),
		)
	}

	expectedUpfrontCount := 0
	if first.UpfrontAmountMinor > 0 {
		expectedUpfrontCount = 1
	}

	if upfrontCount != expectedUpfrontCount {
		return newReconciliationError(
			model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
			fmt.Errorf("%s: checkout payment attempt snapshot has an inconsistent upfront item count", op),
		)
	}

	matchedCharges := 0
	for _, charge := range charges {
		if !model.IsCustomerPayableCharge(charge) {
			continue
		}

		item, exists := snapshotCharges[charge.ID.UUID()]
		if !exists {
			return newReconciliationError(
				model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
				fmt.Errorf("%s: customer-payable first-installment charge is missing from the checkout snapshot", op),
			)
		}

		effect, effectErr := charge.ChargeType.CustomerPaymentChargeEffect()
		if effectErr != nil || item.AmountMinor != charge.AmountMinor || item.Effect != effect {
			return newReconciliationError(
				model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
				fmt.Errorf("%s: checkout payment attempt charge snapshot is inconsistent", op),
			)
		}

		matchedCharges++
	}

	if matchedCharges != len(snapshotCharges) {
		return newReconciliationError(
			model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
			fmt.Errorf("%s: checkout payment attempt snapshot references missing or non-payable charges", op),
		)
	}

	return nil
}

// classifyFirstInstallmentChargesForActivation validates every locked first-installment charge
// row and returns the exact paid/applied transition IDs for a new checkout activation.
//
// It does not access repositories or mutate rows. Every charge that participates in
// activation must currently be offered; incompatible statuses are classified as
// reconciliation_required before any domain mutation can begin.
func classifyFirstInstallmentChargesForActivation(
	firstInstallment model.Installment,
	contractCurrency model.Currency,
	charges []model.InstallmentCharge,
) (firstInstallmentChargeTransitions, error) {
	const op = "contractuc.classifyFirstInstallmentChargesForActivation"

	transitions := firstInstallmentChargeTransitions{
		PaidChargeIDs:    make([]model.InstallmentChargeID, 0, len(charges)),
		AppliedChargeIDs: make([]model.InstallmentChargeID, 0, len(charges)),
	}

	for _, charge := range charges {
		if charge.InstallmentID != firstInstallment.ID {
			return firstInstallmentChargeTransitions{}, newReconciliationError(
				model.InvalidInstallmentStatePaymentReconciliationReason,
				fmt.Errorf("%s: installment charge does not belong to the first installment", op),
			)
		}

		if charge.AmountMinor == 0 {
			return firstInstallmentChargeTransitions{}, newReconciliationError(
				model.InvalidInstallmentStatePaymentReconciliationReason,
				fmt.Errorf("%s: installment charge amount must be positive", op),
			)
		}

		if charge.Currency != contractCurrency || charge.Currency != firstInstallment.Currency {
			return firstInstallmentChargeTransitions{}, newReconciliationError(
				model.PaymentCurrencyMismatchPaymentReconciliationReason,
				fmt.Errorf("%s: installment charge currency does not match contract and first installment", op),
			)
		}

		if err := validateFirstInstallmentChargeStatusForActivation(charge); err != nil {
			return firstInstallmentChargeTransitions{}, err
		}

		bucket, classifyErr := classifyFirstInstallmentChargePolicy(charge)
		if classifyErr != nil {
			return firstInstallmentChargeTransitions{}, classifyErr
		}

		switch bucket {
		case firstInstallmentChargeBucketCustomerPaid:
			transitions.PaidChargeIDs = append(transitions.PaidChargeIDs, charge.ID)

		case firstInstallmentChargeBucketApplied:
			transitions.AppliedChargeIDs = append(transitions.AppliedChargeIDs, charge.ID)
		}
	}

	transitions.PaidChargeIDs = dedupeInstallmentChargeIDs(transitions.PaidChargeIDs)
	transitions.AppliedChargeIDs = dedupeInstallmentChargeIDs(transitions.AppliedChargeIDs)

	return transitions, nil
}

// validateFirstInstallmentChargeStatusForActivation requires every charge participating in a
// new checkout activation to still be offered. Incompatible lifecycle states are classified
// as reconciliation before mutation so the consumer does not retry a permanent inconsistency.
func validateFirstInstallmentChargeStatusForActivation(charge model.InstallmentCharge) error {
	const op = "contractuc.validateFirstInstallmentChargeStatusForActivation"

	if charge.ChargeStatus == model.OfferedInstallmentChargeStatus {
		return nil
	}

	return newReconciliationError(
		model.FirstInstallmentChargeStateInconsistentPaymentReconciliationReason,
		fmt.Errorf(
			"%s: first installment charge %s has incompatible status %q for activation", op, charge.ID,
			charge.ChargeStatus,
		),
	)
}

type firstInstallmentChargeBucket int

const (
	firstInstallmentChargeBucketCustomerPaid firstInstallmentChargeBucket = iota
	firstInstallmentChargeBucketApplied
)

// classifyFirstInstallmentChargePolicy validates the exact (charge type, payer, recipient)
// tuple against the supported first-installment checkout activation policy.
//
// Current production offer creation (decisionuc) emits only:
//
//	bnpl_fee, customer payer, platform recipient
//
// Enum constants alone do not make a combination supported; only tuples with defined
// accounting semantics for initial checkout activation are accepted.
func classifyFirstInstallmentChargePolicy(
	charge model.InstallmentCharge,
) (firstInstallmentChargeBucket, error) {
	const op = "contractuc.classifyFirstInstallmentChargePolicy"

	switch {
	case charge.ChargeType == model.BNPLFeeInstallmentChargeType &&
		charge.PayerType == model.CustomerInstallmentChargePayerType &&
		charge.RecipientType == model.PlatformInstallmentChargeRecipientType:
		return firstInstallmentChargeBucketCustomerPaid, nil

	default:
		return 0, newReconciliationError(
			model.FirstInstallmentChargePolicyInconsistentPaymentReconciliationReason,
			fmt.Errorf(
				"%s: unsupported first installment charge policy: charge_id=%s charge_type=%q payer_type=%q recipient_type=%q",
				op,
				charge.ID,
				charge.ChargeType,
				charge.PayerType,
				charge.RecipientType,
			),
		)
	}
}

// validateFirstInstallmentMonetaryEquality asserts that the incoming paid amount, contract
// payable-now, first installment total, and computed customer payable are all exactly equal
// in the contract currency.
func validateFirstInstallmentMonetaryEquality(
	contractRow model.Contract,
	first model.Installment,
	computedPayable uint64,
	input ActivateFromFinancialCorePaymentResultInput,
) error {
	const op = "contractuc.validateFirstInstallmentMonetaryEquality"

	if input.Currency != contractRow.Currency || input.Currency != first.Currency {
		return newReconciliationError(
			model.PaymentCurrencyMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: payment currency does not match contract and first installment currency", op),
		)
	}

	if computedPayable != first.TotalAmountMinor {
		return newReconciliationError(
			model.PaymentAmountMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: computed first installment payable does not match installment total", op),
		)
	}

	if first.TotalAmountMinor != contractRow.PayableNowAmountMinor {
		return newReconciliationError(
			model.PaymentAmountMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: first installment total does not match contract payable now amount", op),
		)
	}

	if input.PaidAmountMinor != contractRow.PayableNowAmountMinor {
		return newReconciliationError(
			model.PaymentAmountMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: paid amount does not match contract payable now amount", op),
		)
	}

	return nil
}

func validateActivateFromFinancialCorePaymentResultInput(
	input ActivateFromFinancialCorePaymentResultInput,
) error {
	if input.ContractID.IsZero() &&
		(input.SourceType == "" || input.SourceID == "") {
		return contractIDOrPaymentResultSourceRequiredError()
	}

	if input.InvoiceID == "" {
		return invoiceIDRequiredError()
	}

	if input.PaymentIntentID == "" {
		return paymentIntentIDRequiredError()
	}

	if input.PaymentSessionID == "" {
		return paymentSessionIDRequiredError()
	}

	if input.PaidAmountMinor == 0 {
		return paidAmountMinorRequiredError()
	}

	if input.Currency == "" {
		return currencyRequiredError()
	}

	if input.PaidAt.IsZero() {
		return paidAtRequiredError()
	}

	if input.EventID == "" {
		return eventIDRequiredError()
	}

	if input.EventOccurredAt.IsZero() {
		return eventOccurredAtRequiredError()
	}

	return nil
}

func validateInstallmentsForActivation(
	contractRow model.Contract,
	installments []model.Installment,
	first model.Installment,
) error {
	const op = "contractuc.validateInstallmentsForActivation"

	if len(installments) == 0 {
		return cerr.InternalError(fmt.Errorf("%s: installments are missing", op))
	}

	for _, installment := range installments {
		if installment.ContractID == nil || *installment.ContractID != contractRow.ID {
			return cerr.InternalError(
				fmt.Errorf("%s: installment does not belong to contract", op),
			)
		}

		if installment.Status != model.OfferedInstallmentStatus {
			return cerr.InternalError(
				fmt.Errorf("%s: installment status does not allow activation", op),
			)
		}

		if installment.PrincipalAmountMinor == 0 || installment.TotalAmountMinor == 0 {
			return cerr.InternalError(fmt.Errorf("%s: installment amounts must be positive", op))
		}
	}

	if err := validateInstallmentSchedule(
		installments,
		contractRow.InstallmentCount,
		contractRow.ApprovedFinancedAmountMinor,
	); err != nil {
		return err
	}

	if first.Status != model.OfferedInstallmentStatus {
		return cerr.InternalError(
			fmt.Errorf("%s: first installment status does not allow activation", op),
		)
	}

	return nil
}
