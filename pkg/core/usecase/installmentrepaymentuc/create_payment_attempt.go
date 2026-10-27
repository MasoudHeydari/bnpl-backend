package installmentrepaymentuc

import (
	"context"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreatePaymentAttemptInput carries installment repayment initiation inputs.
//
// ExternalCustomerID is the authenticated Identity Service subject ID
// (x-auth-subject-id / customer_profiles.external_customer_id). Ownership checks use the
// resolved customer_profiles.id against contracts.customer_id.
type CreatePaymentAttemptInput struct {
	// TenantID identifies the tenant.
	TenantID model.TenantID
	// ContextID identifies the payment context.
	ContextID model.ContextID

	// ContractID identifies the contract.
	ContractID model.ContractID
	// InstallmentID identifies the installment.
	InstallmentID model.InstallmentID

	// ExternalCustomerID is the Identity Service subject mapped to customer_profiles.external_customer_id.
	ExternalCustomerID model.ExternalCustomerID

	// IdempotencyKey is the client idempotency key for the operation.
	IdempotencyKey string
	// Channel is the payment initiation channel.
	Channel model.PaymentChannel
	// ProviderCode optionally selects the Financial Core checkout payment provider.
	ProviderCode string

	// SelectPriority, when true, ignores ContractID and InstallmentID and
	// selects the current cross-contract priority installment after locking the
	// customer profile inside the payment transaction. Used by the reminder
	// repayment flow so selection and attempt creation share one lock scope.
	SelectPriority bool
}

// CreatePaymentAttemptOutput reports installment repayment initiation results.
type CreatePaymentAttemptOutput struct {
	// Result reports the use-case outcome classification.
	Result CreatePaymentAttemptResult
	// PaymentAttempt is the created or replayed payment attempt.
	PaymentAttempt model.PaymentAttempt
}

// CreatePaymentAttempt queues a BNPL installment repayment through the transactional outbox.
func (uc *UseCase) CreatePaymentAttempt(
	ctx context.Context,
	input CreatePaymentAttemptInput,
) (CreatePaymentAttemptOutput, error) {
	const op = "installmentrepaymentuc.UseCase.CreatePaymentAttempt"

	if err := validateCreatePaymentAttemptInput(input); err != nil {
		return CreatePaymentAttemptOutput{}, err
	}

	now := uc.clock.Now().UTC()

	var (
		paymentAttempt model.PaymentAttempt
		result         CreatePaymentAttemptResult
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					customerProfileQ := uc.customerProfile.Tx(tx)
					contractQ := uc.contract.Tx(tx)
					installmentQ := uc.installment.Tx(tx)
					installmentChargeQ := uc.installmentCharge.Tx(tx)
					paymentAttemptQ := uc.paymentAttempt.Tx(tx)
					outboxQ := uc.outbox.Tx(tx)

					profile, profileErr := customerProfileQ.LockByExternalCustomerID(ctx, input.ExternalCustomerID)
					if profileErr != nil {
						if cerr.IsCode(profileErr, cerr.CodeResourceNotFound) {
							return contractDoesNotBelongToCustomerError(profileErr)
						}

						return profileErr
					}

					customerID := profile.ID

					if input.SelectPriority {
						page := pagination.KeysetRequest{Limit: 200, Order: pagination.OrderAsc}
						page.Normalize()

						unpaid, unpaidErr := installmentQ.ListUnpaidByCustomerID(ctx, customerID, page)
						if unpaidErr != nil {
							return unpaidErr
						}

						selected := selectPriorityInstallment(unpaid, now)
						if selected == nil || selected.ContractID == nil {
							return cerr.ResourceNotFound(
								fmt.Errorf("%s: no payable installment", op),
							)
						}

						input.ContractID = *selected.ContractID
						input.InstallmentID = selected.ID
					}

					existing, existingErr := paymentAttemptQ.GetByInstallmentIDAndIdempotencyKeyWithItems(
						ctx,
						input.InstallmentID,
						input.IdempotencyKey,
					)
					if existingErr == nil {
						contractRow, lockErr := contractQ.LockByID(ctx, input.ContractID)
						if lockErr != nil {
							return lockErr
						}

						if contractRow.CustomerID != customerID {
							return contractDoesNotBelongToCustomerError(
								fmt.Errorf("%s: contract does not belong to customer profile", op),
							)
						}

						fingerprint := paymentAttemptRequestFingerprint(
							input.ContractID,
							input.InstallmentID,
							customerID,
							contractRow.Currency,
							input.Channel,
							paymentAttemptFingerprintContext{
								TenantID:     input.TenantID.String(),
								ContextID:    input.ContextID.String(),
								ProviderCode: input.ProviderCode,
							},
						)
						if existing.RequestFingerprint != fingerprint {
							return idempotencyKeyConflictError(
								fmt.Errorf("%s: idempotency key reused with different payment inputs", op),
							)
						}

						paymentAttempt = existing
						result = CreatePaymentAttemptResultAlreadyExists

						return nil
					}

					if !cerr.IsCode(existingErr, cerr.CodeResourceNotFound) {
						return existingErr
					}

					contractRow, lockErr := contractQ.LockByID(ctx, input.ContractID)
					if lockErr != nil {
						return lockErr
					}

					if contractRow.CustomerID != customerID {
						return contractDoesNotBelongToCustomerError(
							fmt.Errorf("%s: contract does not belong to customer profile", op),
						)
					}

					if contractRow.Status == model.CancelingContractStatus {
						return contractIsCancelingError()
					}

					if contractRow.Status != model.ActiveContractStatus {
						return contractNotActiveError()
					}

					installmentRow, lockErr := installmentQ.LockByID(ctx, input.InstallmentID)
					if lockErr != nil {
						return lockErr
					}

					if !installmentBelongsToContract(installmentRow, input.ContractID) {
						return installmentDoesNotBelongToContractError()
					}

					if !isPayableInstallmentStatus(installmentRow.Status) {
						return installmentStatusDoesNotAllowPaymentError()
					}

					siblings, siblingsErr := installmentQ.ListByContractID(ctx, input.ContractID)
					if siblingsErr != nil {
						return siblingsErr
					}

					if !model.IsNextPayableInSequence(installmentRow, siblings) {
						return installmentNotNextPayableError()
					}

					charges, chargesErr := installmentChargeQ.ListByInstallmentIDs(
						ctx,
						[]model.InstallmentID{installmentRow.ID},
					)
					if chargesErr != nil {
						return chargesErr
					}

					payableAmount, payableOK, payableErr := model.CustomerPayableAmount(installmentRow, charges)
					if payableErr != nil {
						return payableErr
					}

					if !payableOK || payableAmount == 0 {
						return installmentPayableAmountZeroError()
					}

					if amountErr := model.ValidatePositiveAmountMinor(payableAmount); amountErr != nil {
						// Programming invariant: payable amount was already confirmed non-zero.
						return cerr.InternalError(fmt.Errorf("%s: installment payable amount: %w", op, amountErr))
					}

					// The receivable created during activation currently represents financed principal only.
					// Until FC exposes recognition/application contracts for charges, do not silently
					// apply fees or penalties to the principal receivable.
					if payableAmount != installmentRow.PrincipalAmountMinor {
						return installmentChargesRequireFinancialRecognitionError()
					}

					snapshotItems, snapshotErr := model.BuildPaymentAttemptItems(installmentRow, charges)
					if snapshotErr != nil {
						return snapshotErr
					}

					if itemTotal, ok := model.PayableAmountFromItems(snapshotItems); !ok || itemTotal != payableAmount {
						// Programming invariant: snapshot items must match computed payable amount.
						return cerr.InternalError(fmt.Errorf("%s: payment attempt snapshot total is inconsistent", op))
					}

					fingerprint := paymentAttemptRequestFingerprint(
						input.ContractID,
						input.InstallmentID,
						customerID,
						contractRow.Currency,
						input.Channel,
						paymentAttemptFingerprintContext{
							TenantID:     input.TenantID.String(),
							ContextID:    input.ContextID.String(),
							ProviderCode: input.ProviderCode,
						},
					)

					created, createErr := paymentAttemptQ.CreateWithItems(
						ctx,
						input.ContractID,
						input.InstallmentID,
						payableAmount,
						contractRow.Currency,
						model.CreatedPaymentAttemptState,
						input.Channel,
						input.IdempotencyKey,
						fingerprint,
						snapshotItems,
						now,
						now,
					)
					if createErr != nil {
						if cerr.IsCode(createErr, cerr.CodeConflict) {
							return activePaymentAttemptExistsError(createErr)
						}

						return createErr
					}

					providerCode := optionalTrimmedString(input.ProviderCode)
					bound, bindErr := paymentAttemptQ.BindFinancialRequestContextIfAbsent(
						ctx,
						created.ID,
						input.TenantID.String(),
						input.ContextID.String(),
						providerCode,
						uc.paymentReturnURL,
						uc.paymentReturnURL,
						uc.paymentReturnURL,
						now,
					)
					if bindErr != nil {
						return bindErr
					}

					bound.Items = created.Items
					created = bound

					if err := uc.insertCreatePaymentAttemptAuditOutbox(
						ctx,
						outboxQ,
						created,
						installmentRepaymentAuditFacts{
							ExternalCustomerID: input.ExternalCustomerID,
							CustomerID:        customerID,
							Installment:       installmentRow,
							Charges:           charges,
						},
						now,
					); err != nil {
						return err
					}

					paymentAttempt = created
					result = CreatePaymentAttemptResultQueued

					return nil
				},
			)
		},
	)
	if err != nil {
		return CreatePaymentAttemptOutput{}, err
	}

	paymentAttempt, err = uc.ensureFinancialCoreInstallmentPayment(ctx, paymentAttempt, input)
	if err != nil {
		return CreatePaymentAttemptOutput{}, err
	}

	return CreatePaymentAttemptOutput{
		Result:         result,
		PaymentAttempt: paymentAttempt,
	}, nil
}

const (
	maxInstallmentIdempotencyKeyLength = 256
)

func validateCreatePaymentAttemptInput(input CreatePaymentAttemptInput) error {
	if input.TenantID.IsZero() {
		return tenantIDRequiredError()
	}

	if input.ContextID.IsZero() {
		return contextIDRequiredError()
	}

	if !input.SelectPriority {
		if input.ContractID.IsZero() {
			return contractIDRequiredError()
		}

		if input.InstallmentID.IsZero() {
			return installmentIDRequiredError()
		}
	}

	if input.ExternalCustomerID.IsZero() {
		return externalCustomerIDRequiredError()
	}

	if strings.TrimSpace(input.IdempotencyKey) == "" ||
		len(strings.TrimSpace(input.IdempotencyKey)) > maxInstallmentIdempotencyKeyLength {
		return idempotencyKeyInvalidError()
	}

	if !input.Channel.IsValid() {
		return paymentChannelInvalidError()
	}

	if len(strings.TrimSpace(input.ProviderCode)) > 128 {
		return providerCodeTooLongError()
	}

	return nil
}
