package contractuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// createPaymentPending creates a payment_pending contract for policies that
// require an initial checkout payment before activation, and attaches offered
// installments to that contract.
func (uc *UseCase) createPaymentPending(
	ctx context.Context,
	input CreateContractInput,
	submitPayment bool,
) (CreateContractOutput, error) {
	const op = "contractuc.UseCase.createPaymentPending"

	if err := validatePaymentBasedCreateContractInput(input); err != nil {
		return CreateContractOutput{}, err
	}

	now := uc.clock.Now().UTC()
	var output CreateContractOutput

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					customerProfileQ := uc.customerProfile.Tx(tx)
					allocationQ := uc.checkoutDecisionAllocation.Tx(tx)
					contractQ := uc.contract.Tx(tx)
					creditAccountQ := uc.creditAccount.Tx(tx)
					installmentQ := uc.installment.Tx(tx)
					installmentChargeQ := uc.installmentCharge.Tx(tx)
					paymentAttemptQ := uc.paymentAttempt.Tx(tx)
					auditOutboxQ := uc.outbox.Tx(tx)

					profile, profileErr := customerProfileQ.LockByExternalCustomerID(ctx, input.ExternalCustomerID)
					if profileErr != nil {
						if cerr.IsCode(profileErr, cerr.CodeResourceNotFound) {
							return checkoutDecisionDoesNotBelongToCustomerError(
								fmt.Errorf("%s: checkout decision does not belong to customer profile", op),
							)
						}

						return profileErr
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

					if allocation.CustomerID != profile.ID {
						return checkoutDecisionDoesNotBelongToCustomerError(
							fmt.Errorf("%s: checkout decision does not belong to customer profile", op),
						)
					}

					installments, err := installmentQ.LockByDecisionID(ctx, input.DecisionID)
					if err != nil {
						return err
					}

					first, ok := firstInstallment(installments)
					if !ok {
						return cerr.InternalError(fmt.Errorf("%s: first installment is missing", op))
					}

					payableNow := first.TotalAmountMinor
					remainingPrincipal := allocation.ApprovedFinancedAmountMinor - first.PrincipalAmountMinor

					// Resolve an idempotent replay before offer-expiry and credit
					// checks. A previously-created contract remains the same logical
					// operation even if the offer expired or credit capacity changed.
					existingContract, existingErr := contractQ.GetByDecisionID(ctx, input.DecisionID)
					if existingErr == nil {
						existingOutput, outputErr := outputFromExistingContract(existingContract)
						if outputErr != nil {
							return outputErr
						}

						attempt, attemptErr := uc.ensureCheckoutPaymentAttemptForFirstInstallment(
							ctx,
							paymentAttemptQ,
							installmentChargeQ,
							input,
							existingContract,
							first,
							now,
						)
						if attemptErr != nil {
							return attemptErr
						}

						existingOutput.PaymentAttempt = attempt
						output = existingOutput
						return nil
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

					// New Contract path: enforce offer validity and schedule shape.
					if err = validateAllocationForContract(profile.ID, allocation, now); err != nil {
						return err
					}

					if err = validateOfferedInstallmentsForContract(installments); err != nil {
						return err
					}

					if err = validateInstallmentSchedule(
						installments,
						allocation.InstallmentCount,
						allocation.ApprovedFinancedAmountMinor,
					); err != nil {
						return err
					}

					// Serialize the start of a credit-consuming payment flow on the single
					// CreditAccount row. No amount is reserved: the account continues to
					// expose only committed principal. The payment_pending Contract created
					// below is the durable concurrency gate.
					account, accountErr := creditAccountQ.LockByCustomerIDAndCurrency(ctx, profile.ID, allocation.Currency)
					if accountErr != nil {
						return accountErr
					}

					if !account.IsUsableAt(now) {
						return creditAccountNotUsableForContractError()
					}

					if account.AvailableAmountMinor() < remainingPrincipal {
						return insufficientCreditForContractError(account.AvailableAmountMinor(), remainingPrincipal)
					}

					contractRow, created, createErr := contractQ.Create(
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
						payableNow,
						allocation.ApprovedFinancedAmountMinor,
						model.PaymentPendingContractStatus,
						allocation.ExpiresAt,
						now,
						now,
					)
					if createErr != nil {
						return createErr
					}

					if !created {
						// ON CONFLICT DO NOTHING intentionally lets PostgreSQL arbitrate all
						// concurrent creators. Distinguish an idempotent replay of this
						// decision from another checkout already occupying the account's
						// single in-flight-payment slot.
						if sameDecision, reloadErr := contractQ.GetByDecisionID(ctx, input.DecisionID); reloadErr == nil {
							existingOutput, outputErr := outputFromExistingContract(sameDecision)
							if outputErr != nil {
								return outputErr
							}

							attempt, attemptErr := uc.ensureCheckoutPaymentAttemptForFirstInstallment(
								ctx,
								paymentAttemptQ,
								installmentChargeQ,
								input,
								sameDecision,
								first,
								now,
							)
							if attemptErr != nil {
								return attemptErr
							}

							existingOutput.PaymentAttempt = attempt
							output = existingOutput
							return nil
						} else if !cerr.IsCode(reloadErr, cerr.CodeResourceNotFound) {
							return reloadErr
						}

						pending, pendingErr := contractQ.GetPaymentPendingByCreditAccountID(ctx, account.ID)
						if pendingErr == nil {
							return paymentAlreadyInProgressError(pending.ID)
						}

						if !cerr.IsCode(pendingErr, cerr.CodeResourceNotFound) {
							return pendingErr
						}

						return cerr.Conflict(fmt.Errorf("%s: contract create conflict could not be resolved", op))
					}

					affected, attachErr := installmentQ.AttachContractToOfferedByDecisionID(
						ctx,
						input.DecisionID,
						contractRow.ID,
						now,
					)
					if attachErr != nil {
						return attachErr
					}

					if affected != int64(len(installments)) {
						return cerr.InternalError(
							fmt.Errorf("%s: offered installments could not be attached to contract", op),
						)
					}

					firstCharges, chargesErr := installmentChargeQ.ListByInstallmentIDs(
						ctx,
						[]model.InstallmentID{first.ID},
					)
					if chargesErr != nil {
						return chargesErr
					}

					attempt, attemptErr := uc.ensureCheckoutPaymentAttemptInTx(
						ctx,
						paymentAttemptQ,
						input,
						contractRow,
						first,
						firstCharges,
						now,
					)
					if attemptErr != nil {
						return attemptErr
					}

					output = CreateContractOutput{
						Result:         CreateContractResultCreated,
						Contract:       contractRow,
						PaymentAttempt: attempt,
					}

					if err = uc.insertCreatePaymentPendingAuditOutbox(
						ctx,
						auditOutboxQ,
						createPaymentPendingAuditFacts{
							ExternalCustomerID: input.ExternalCustomerID,
							Contract:           contractRow,
							Installments:       installments,
							InstallmentCharges: firstCharges,
							PaymentAttempt:     attempt,
						},
						now,
					); err != nil {
						return err
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return CreateContractOutput{}, err
	}

	output.PaymentRequired = true

	if !submitPayment {
		// Local preparation is committed. CheckoutSession will link the Contract
		// before any external payment submission is allowed.
		return output, nil
	}

	return uc.SubmitPreparedCheckoutFulfillment(ctx, output)
}

const (
	maxProviderCodeLength        = 128
	maxFinancialIdentifierLength = 256
	maxReturnURLLength           = 2048
)

func validateCreateContractInput(input CreateContractInput) error {
	if input.DecisionID.IsZero() {
		return decisionIDRequiredError()
	}

	if input.ExternalCustomerID.IsZero() {
		return externalCustomerIDRequiredError()
	}

	return nil
}

// validatePaymentBasedCreateContractInput validates fields consumed only by the
// Financial Core checkout-payment path. Direct-activation policies deliberately
// do not require payment-specific tenant/context/provider input.
func validatePaymentBasedCreateContractInput(input CreateContractInput) error {
	if err := validateCreateContractInput(input); err != nil {
		return err
	}

	providerCode := strings.TrimSpace(input.ProviderCode)
	if len(providerCode) > maxProviderCodeLength {
		return providerCodeTooLongError()
	}

	return nil
}

func validateAllocationForContract(
	customerID model.CustomerID,
	allocation model.CheckoutDecisionAllocation,
	now time.Time,
) error {
	const op = "contractuc.validateAllocationForContract"

	if allocation.CustomerID != customerID {
		return checkoutDecisionDoesNotBelongToCustomerError(
			fmt.Errorf("%s: checkout decision does not belong to customer profile", op),
		)
	}

	if !allocation.ExpiresAt.After(now) {
		return checkoutDecisionAllocationExpiredError()
	}

	return nil
}

func validateOfferedInstallmentsForContract(installments []model.Installment) error {
	const op = "contractuc.validateOfferedInstallmentsForContract"

	if len(installments) == 0 {
		return cerr.InternalError(fmt.Errorf("%s: offered installments are missing", op))
	}

	for _, installment := range installments {
		switch installment.Status {
		case model.OfferedInstallmentStatus:
			continue

		default:
			return installmentStatusDoesNotAllowContractCreationError()
		}
	}

	return nil
}

func outputFromExistingContract(contractRow model.Contract) (CreateContractOutput, error) {
	switch contractRow.Status {
	case model.PaymentPendingContractStatus:
		return CreateContractOutput{
			Result:          CreateContractResultAlreadyExists,
			Contract:        contractRow,
			PaymentRequired: true,
		}, nil

	case model.ActiveContractStatus:
		return CreateContractOutput{}, contractAlreadyActiveError()

	default:
		return CreateContractOutput{}, contractStatusDoesNotAllowCreationError()
	}
}

func (uc *UseCase) ensureCheckoutPaymentAttemptForFirstInstallment(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	input CreateContractInput,
	contractRow model.Contract,
	first model.Installment,
	now time.Time,
) (model.PaymentAttempt, error) {
	firstCharges, err := installmentChargeQ.ListByInstallmentIDs(ctx, []model.InstallmentID{first.ID})
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return uc.ensureCheckoutPaymentAttemptInTx(
		ctx,
		paymentAttemptQ,
		input,
		contractRow,
		first,
		firstCharges,
		now,
	)
}

func isPlausibleContractCreateConflict(err error) bool {
	if err == nil {
		return false
	}

	return cerr.IsCode(err, cerr.CodeValidationFailed) || cerr.IsCode(err, cerr.CodeConflict)
}
