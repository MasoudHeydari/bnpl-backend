package contractuc

import (
	"context"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// merchantFundingPreparation is the result of durably preparing a
// merchant-funding contract. The contract is payment_pending until Financial
// Core merchant funding succeeds.
type merchantFundingPreparation struct {
	Contract model.Contract
	Result   CreateContractResult
}

// createMerchantFundingContract handles ActivationAfterMerchantFunding
// policies. It creates a payment_pending contract (no PaymentAttempt, no
// customer cash) and, when submitRecognition is set, records merchant
// entitlement with Financial Core; the contract becomes active only after that
// Financial Core success, never merely because it was created locally.
func (uc *UseCase) createMerchantFundingContract(
	ctx context.Context,
	input CreateContractInput,
	selected checkout.Policy,
	submitRecognition bool,
) (CreateContractOutput, error) {
	prepared, err := uc.prepareMerchantFunding(ctx, input, selected)
	if err != nil {
		return CreateContractOutput{}, err
	}

	result := CreateContractResultCreated
	if prepared.Result != "" {
		result = prepared.Result
	}

	output := CreateContractOutput{
		Result:          result,
		Contract:        prepared.Contract,
		PaymentRequired: false,
	}

	// Checkout Confirm must durably link the Contract before any Financial Core
	// call. PrepareCheckoutContract stops after local persistence.
	if !submitRecognition {
		return output, nil
	}

	return uc.submitPreparedMerchantFunding(ctx, output)
}

// submitPreparedMerchantFunding records merchant entitlement with Financial
// Core and finalizes local activation of a payment_pending merchant-funding
// contract. It must run only after CheckoutSession has committed its contract
// link. No PaymentAttempt is involved in this path.
func (uc *UseCase) submitPreparedMerchantFunding(
	ctx context.Context,
	prepared CreateContractOutput,
) (CreateContractOutput, error) {
	const op = "contractuc.UseCase.submitPreparedMerchantFunding"

	contract := prepared.Contract

	// Compatibility recovery for contracts created by older code that marked the
	// contract active before Financial Core recognition.
	if contract.Status == model.ActiveContractStatus {
		if err := uc.EnsureMerchantEntitlement(ctx, contract.ID); err != nil {
			return CreateContractOutput{}, err
		}

		contractRow, loadErr := uc.loadContract(ctx, contract.ID)
		if loadErr != nil {
			return CreateContractOutput{}, loadErr
		}

		return CreateContractOutput{
			Result:          CreateContractResultAlreadyExists,
			Contract:        contractRow,
			PaymentRequired: false,
		}, nil
	}

	if contract.Status != model.PaymentPendingContractStatus {
		return CreateContractOutput{}, fmt.Errorf(
			"%s: contract status %q is not payment_pending",
			op,
			contract.Status,
		)
	}

	var (
		selected           checkout.Policy
		externalCustomerID model.ExternalCustomerID
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		decisionQ := uc.decision.Conn(conn)
		customerProfileQ := uc.customerProfile.Conn(conn)

		decision, findErr := decisionQ.GetByID(ctx, contract.DecisionID)
		if findErr != nil {
			return findErr
		}

		profile, profileErr := customerProfileQ.GetByID(ctx, contract.CustomerID)
		if profileErr != nil {
			return profileErr
		}

		externalCustomerID = profile.ExternalCustomerID

		var resolveErr error
		selected, resolveErr = uc.checkoutPolicies.Resolve(policy.Ref{
			ID:      decision.PolicyID,
			Version: decision.PolicyVersion,
		})

		return resolveErr
	})
	if err != nil {
		return CreateContractOutput{}, fmt.Errorf("%s: resolve checkout context: %w", op, err)
	}

	recorded, err := uc.recordMerchantEntitlement(
		ctx,
		contract.ID,
		model.PaymentPendingContractStatus,
	)
	if err != nil {
		return CreateContractOutput{}, err
	}

	merchantFundingPrep := merchantFundingPreparation{
		Contract: contract,
		Result:   prepared.Result,
	}

	output, activatedNow, err := uc.finalizeMerchantFundingActivation(
		ctx,
		CreateContractInput{
			DecisionID:         contract.DecisionID,
			ExternalCustomerID: externalCustomerID,
		},
		merchantFundingPrep,
		selected,
		recorded,
	)
	if err != nil {
		return CreateContractOutput{}, err
	}

	if activatedNow {
		uc.businessMetrics.ContractCreated(ctx, string(output.Contract.Status))
	}

	return output, nil
}

// prepareMerchantFunding validates and durably creates the local pre-activation
// contract. No Financial Core call occurs while the database transaction is
// open, and no credit exposure is consumed while the contract is
// payment_pending (waiting for Financial Core merchant funding).
func (uc *UseCase) prepareMerchantFunding(
	ctx context.Context,
	input CreateContractInput,
	selected checkout.Policy,
) (merchantFundingPreparation, error) {
	const op = "contractuc.UseCase.prepareMerchantFunding"
	now := uc.clock.Now().UTC()
	var prepared merchantFundingPreparation

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			profileQ := uc.customerProfile.Tx(tx)
			decisionQ := uc.decision.Tx(tx)
			allocationQ := uc.checkoutDecisionAllocation.Tx(tx)
			contractQ := uc.contract.Tx(tx)
			creditAccountQ := uc.creditAccount.Tx(tx)
			installmentQ := uc.installment.Tx(tx)

			profile, err := profileQ.LockByExternalCustomerID(ctx, input.ExternalCustomerID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return checkoutDecisionDoesNotBelongToCustomerError(
						fmt.Errorf("%s: checkout decision does not belong to customer profile", op),
					)
				}

				return err
			}

			// A user-triggered replay is allowed to resume a previously prepared
			// merchant-funding activation without requiring the original decision
			// TTL to still be live. Its economics and policy identity were fixed
			// when the durable payment_pending contract was created.
			existing, existingErr := contractQ.GetByDecisionID(ctx, input.DecisionID)
			if existingErr == nil {
				if existing.CustomerID != profile.ID {
					return checkoutDecisionDoesNotBelongToCustomerError(
						fmt.Errorf("%s: existing contract does not belong to customer profile", op),
					)
				}

				switch existing.Status {
				case model.PaymentPendingContractStatus, model.ActiveContractStatus:
					prepared = merchantFundingPreparation{
						Contract: existing,
						Result:   CreateContractResultAlreadyExists,
					}
					return nil

				default:
					return contractStatusDoesNotAllowCreationError()
				}
			}

			if !cerr.IsCode(existingErr, cerr.CodeResourceNotFound) {
				return existingErr
			}

			// New credit commitment boundary: existing-contract replay above
			// remains possible after a collections restriction takes effect.
			if guardErr := uc.newCreditUsageGuard.AssertNewCreditAllowed(
				ctx,
				tx,
				profile.ID,
				now,
			); guardErr != nil {
				return guardErr
			}

			decision, err := decisionQ.LockLiveCheckoutDecisionByID(ctx, input.DecisionID, now)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					// createContract loaded this immutable Decision before entering
					// the policy-specific path. A miss here means it is no longer
					// live at the final locked Contract boundary.
					return checkoutDecisionAllocationExpiredError()
				}

				return err
			}

			persistedPolicy, err := uc.checkoutPolicies.Resolve(policy.Ref{ID: decision.PolicyID, Version: decision.PolicyVersion})
			if err != nil {
				return fmt.Errorf("%s: resolve persisted policy: %w", op, err)
			}

			if persistedPolicy.Ref() != selected.Ref() {
				return cerr.InternalError(fmt.Errorf("%s: checkout policy changed while creating contract", op))
			}

			allocation, err := allocationQ.LockByDecisionID(ctx, input.DecisionID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return checkoutDecisionAllocationMissingError(
						fmt.Errorf("%s: checkout decision allocation is missing", op),
					)
				}

				return err
			}

			// New first Contract for this Decision is origination.
			// Existing-Contract replay above must remain possible after
			// origination is later disabled for the merchant jurisdiction.
			merchantProfileQ := uc.merchantProfile.Tx(tx)
			merchant, merchantErr := merchantProfileQ.GetByID(ctx, allocation.MerchantProfileID)
			if merchantErr != nil {
				return merchantErr
			}

			if err = uc.requireOriginationEnabledJurisdiction(op, merchant.Jurisdiction); err != nil {
				return err
			}

			if err = validateAllocationForContract(profile.ID, allocation, now); err != nil {
				return err
			}

			installments, err := installmentQ.LockByDecisionID(ctx, input.DecisionID)
			if err != nil {
				return err
			}

			if err = validateOfferedInstallmentsForContract(installments); err != nil {
				return err
			}
			if _, err := validateCashObligations(ctx, uc.installmentCharge.Tx(tx), installments); err != nil {
				return err
			}

			if err = validateInstallmentSchedule(
				installments,
				allocation.InstallmentCount,
				allocation.ApprovedFinancedAmountMinor,
			); err != nil {
				return err
			}

			first, ok := firstInstallment(installments)
			if !ok {
				return cerr.InternalError(fmt.Errorf("%s: first installment is missing", op))
			}

			plan, err := buildContractPlan(selected, allocation, first)
			if err != nil {
				return cerr.InternalError(fmt.Errorf("%s: build contract plan: %w", op, err))
			}

			if plan.InitialPaymentRequired || plan.ActivationMode != checkout.ActivationAfterMerchantFunding {
				return cerr.InternalError(fmt.Errorf("%s: policy contract plan requires initial payment instead of merchant funding", op))
			}

			account, err := creditAccountQ.LockByCustomerIDAndCurrency(ctx, profile.ID, allocation.Currency)
			if err != nil {
				return err
			}

			if !account.IsUsableAt(now) {
				return creditAccountNotUsableForContractError()
			}

			if account.AvailableAmountMinor() < plan.ActiveExposureAmountMinor {
				return insufficientCreditForContractError(account.AvailableAmountMinor(), plan.ActiveExposureAmountMinor)
			}

			contractRow, created, err := contractQ.Create(
				ctx,
				allocation.DecisionID,
				allocation.CheckoutKey,
				allocation.CustomerID,
				account.ID,
				allocation.MerchantProfileID,
				allocation.MerchantBranchProfileID,
				allocation.Currency,
				allocation.RequestedAmountMinor,
				allocation.ApprovedFinancedAmountMinor,
				allocation.UpfrontAmountMinor,
				allocation.InstallmentCount,
				0,
				plan.ReceivableAmountMinor,
				model.PaymentPendingContractStatus,
				allocation.ExpiresAt,
				now,
				now,
			)
			if err != nil {
				return err
			}

			if !created {
				if sameDecision, reloadErr := contractQ.GetByDecisionID(ctx, input.DecisionID); reloadErr == nil {
					if sameDecision.Status != model.PaymentPendingContractStatus &&
						sameDecision.Status != model.ActiveContractStatus {
						return contractStatusDoesNotAllowCreationError()
					}

					prepared = merchantFundingPreparation{
						Contract: sameDecision,
						Result:   CreateContractResultAlreadyExists,
					}
					return nil
				} else if !cerr.IsCode(reloadErr, cerr.CodeResourceNotFound) {
					return reloadErr
				}

				return cerr.Conflict(fmt.Errorf("%s: another pre-activation contract already occupies the credit account", op))
			}

			affected, err := installmentQ.AttachContractToOfferedByDecisionID(
				ctx,
				input.DecisionID,
				contractRow.ID,
				now,
			)
			if err != nil {
				return err
			}

			if affected != int64(len(installments)) {
				return cerr.InternalError(fmt.Errorf("%s: offered installments could not be attached to contract", op))
			}

			prepared = merchantFundingPreparation{
				Contract: contractRow,
				Result:   CreateContractResultCreated,
			}
			return nil
		})
	})
	if err != nil {
		return merchantFundingPreparation{}, err
	}

	return prepared, nil
}

// finalizeMerchantFundingActivation atomically applies the Financial Core recognition
// result and makes the contract locally active. If this transaction fails after
// Financial Core succeeded, the contract remains payment_pending; a later
// user request reuses the same contract source reference and resumes safely.
func (uc *UseCase) finalizeMerchantFundingActivation(
	ctx context.Context,
	input CreateContractInput,
	prepared merchantFundingPreparation,
	selected checkout.Policy,
	recorded merchantEntitlementRecord,
) (CreateContractOutput, bool, error) {
	const op = "contractuc.UseCase.finalizeMerchantFundingActivation"
	now := uc.clock.Now().UTC()
	var (
		output       CreateContractOutput
		activatedNow bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			if err := uc.lockActivationCustomer(ctx, tx, prepared.Contract.ID); err != nil {
				return err
			}
			contractQ := uc.contract.Tx(tx)
			creditAccountQ := uc.creditAccount.Tx(tx)
			installmentQ := uc.installment.Tx(tx)
			installmentChargeQ := uc.installmentCharge.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			contractRow, err := contractQ.LockByID(ctx, prepared.Contract.ID)
			if err != nil {
				return err
			}

			if contractRow.Status == model.ActiveContractStatus {
				if !financialRecognitionMatches(contractRow, recorded) {
					return cerr.Conflict(
						fmt.Errorf("%s: active contract Financial Core recognition conflicts with synchronous result", op),
						map[string]any{"contract_id": contractRow.ID},
					)
				}

				output = CreateContractOutput{
					Result:          CreateContractResultAlreadyExists,
					Contract:        contractRow,
					PaymentRequired: false,
				}
				return nil
			}

			if contractRow.Status != model.PaymentPendingContractStatus {
				return contractStatusDoesNotAllowCreationError()
			}

			if err = contractRow.Status.ValidateTransitionTo(model.ActiveContractStatus); err != nil {
				return cerr.InternalError(err)
			}

			installments, err := installmentQ.LockByContractID(ctx, contractRow.ID)
			if err != nil {
				return err
			}

			if err = validateOfferedInstallmentsForContract(installments); err != nil {
				return err
			}

			if err = validateInstallmentSchedule(
				installments,
				contractRow.InstallmentCount,
				contractRow.ApprovedFinancedAmountMinor,
			); err != nil {
				return err
			}

			first, ok := firstInstallment(installments)
			if !ok {
				return cerr.InternalError(fmt.Errorf("%s: first installment is missing", op))
			}

			plan, err := buildContractPlan(selected, model.CheckoutDecisionAllocation{
				RequestedAmountMinor:        contractRow.RequestedAmountMinor,
				ApprovedFinancedAmountMinor: contractRow.ApprovedFinancedAmountMinor,
				UpfrontAmountMinor:          contractRow.UpfrontAmountMinor,
			}, first)
			if err != nil {
				return cerr.InternalError(fmt.Errorf("%s: rebuild contract plan: %w", op, err))
			}

			if plan.InitialPaymentRequired || plan.ActivationMode != checkout.ActivationAfterMerchantFunding {
				return cerr.InternalError(fmt.Errorf("%s: policy contract plan no longer allows merchant-funding activation", op))
			}

			if plan.ReceivableAmountMinor != contractRow.RemainingPrincipalAmountMinor {
				return cerr.InternalError(fmt.Errorf("%s: contract receivable changed before activation", op))
			}

			// Capacity was validated while the payment_pending gate was created.
			// Lock the same account again to serialize the exposure mutation, but do
			// not re-evaluate mutable credit policy after Financial Core has already
			// accepted the immutable contract source reference.
			account, err := creditAccountQ.LockByID(ctx, contractRow.CreditAccountID)
			if err != nil {
				return err
			}

			if account.CustomerID != contractRow.CustomerID || account.Currency != contractRow.Currency {
				return cerr.InternalError(fmt.Errorf("%s: contract credit account ownership changed before activation", op))
			}

			schedule, err := selected.BuildInstallmentSchedule(checkout.ScheduleInput{
				Now:                 now,
				FinancedAmountMinor: contractRow.ApprovedFinancedAmountMinor,
				UpfrontAmountMinor:  contractRow.UpfrontAmountMinor,
				InstallmentCount:    contractRow.InstallmentCount,
			})
			if err != nil {
				return cerr.InternalError(fmt.Errorf("%s: finalize installment schedule: %w", op, err))
			}

			if len(schedule.Installments) != len(installments) {
				return cerr.InternalError(fmt.Errorf("%s: finalized installment count mismatch", op))
			}

			byOrder := make(map[int]checkout.InstallmentDraft, len(schedule.Installments))
			for _, draft := range schedule.Installments {
				byOrder[draft.Order] = draft
			}
			for _, installment := range installments {
				draft, exists := byOrder[installment.InstallmentOrder]
				if !exists ||
					draft.PrincipalAmountMinor != installment.PrincipalAmountMinor ||
					draft.TotalAmountMinor != installment.TotalAmountMinor {
					return cerr.InternalError(fmt.Errorf("%s: finalized installment economics differ from decision snapshot", op))
				}

				if _, err = installmentQ.UpdateOfferedDueAt(ctx, installment.ID, draft.DueAt, now); err != nil {
					return err
				}
			}

			updatedInstallments := make([]model.Installment, 0, len(installments))
			for _, installment := range installments {
				draft := byOrder[installment.InstallmentOrder]
				nextStatus := model.ScheduledInstallmentStatus
				if !draft.DueAt.After(now) {
					nextStatus = model.DueInstallmentStatus
				}

				updated, updateErr := installmentQ.MarkScheduledOrDue(
					ctx,
					installment.ID,
					contractRow.ID,
					nextStatus,
					now,
				)
				if updateErr != nil {
					return updateErr
				}

				updatedInstallments = append(updatedInstallments, updated)
			}

			ids := installmentIDs(updatedInstallments)
			if _, err = installmentChargeQ.MarkOfferedChargesAppliedByInstallmentIDs(ctx, ids, now); err != nil {
				return err
			}

			if _, err = contractQ.BindMerchantEntitlementIfAbsent(
				ctx,
				contractRow.ID,
				recorded.MerchantEntitlementID,
				now,
			); err != nil {
				return err
			}

			if _, err = contractQ.BindReceivableIfAbsent(
				ctx,
				contractRow.ID,
				recorded.ReceivableID,
				now,
			); err != nil {
				return err
			}

			activated, err := contractQ.MarkActive(
				ctx,
				contractRow.ID,
				nil,
				nil,
				nil,
				plan.ReceivableAmountMinor,
				now,
				now,
			)
			if err != nil {
				return err
			}

			if plan.ActiveExposureAmountMinor > 0 {
				if _, err = creditAccountQ.AddExposure(ctx, account.ID, plan.ActiveExposureAmountMinor, now); err != nil {
					return err
				}

				if _, err = creditAccountQ.AppendTransaction(
					ctx,
					account.ID,
					model.ExposureIncreasedCreditTransactionType,
					plan.ActiveExposureAmountMinor,
					contractRow.Currency,
					model.ContractActivatedCreditTransactionReason,
					model.ContractCreditTransactionReferenceType,
					activated.ID.UUID(),
					now,
					now,
				); err != nil {
					return err
				}
			}

			if err = uc.insertCreateActiveContractAuditOutbox(
				ctx,
				outboxQ,
				input.ExternalCustomerID,
				activated,
				updatedInstallments,
				now,
			); err != nil {
				return err
			}

			activatedNow = true
			output = CreateContractOutput{
				Result:          prepared.Result,
				Contract:        activated,
				PaymentRequired: false,
			}
			return nil
		})
	})
	if err != nil {
		return CreateContractOutput{}, false, err
	}

	return output, activatedNow, nil
}

func (uc *UseCase) loadContract(ctx context.Context, contractID model.ContractID) (model.Contract, error) {
	var contractRow model.Contract
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var err error
		contractQ := uc.contract.Conn(conn)
		contractRow, err = contractQ.GetByID(ctx, contractID)
		return err
	})
	return contractRow, err
}

func financialRecognitionMatches(contractRow model.Contract, recorded merchantEntitlementRecord) bool {
	if !contractHasFinancialRecognition(contractRow) {
		return false
	}

	return strings.TrimSpace(*contractRow.FinancialMerchantEntitlementID) == recorded.MerchantEntitlementID &&
		strings.TrimSpace(*contractRow.FinancialReceivableID) == recorded.ReceivableID
}
