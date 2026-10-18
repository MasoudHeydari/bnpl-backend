package contractuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func (uc *UseCase) ensureCheckoutPaymentAttemptInTx(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	input CreateContractInput,
	contractRow model.Contract,
	firstInstallment model.Installment,
	charges []model.InstallmentCharge,
	now time.Time,
) (model.PaymentAttempt, error) {
	const op = "contractuc.UseCase.ensureCheckoutPaymentAttemptInTx"

	existing, err := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx,
		contractRow.ID,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	retryID := uuid.Nil
	if err == nil {
		if validateErr := validateCheckoutPaymentAttempt(
			existing,
			input,
			contractRow,
			firstInstallment,
			uc.paymentReturnURL,
		); validateErr != nil {
			return model.PaymentAttempt{}, validateErr
		}

		if existing.State != model.FailedPaymentAttemptState && existing.State != model.CanceledPaymentAttemptState {
			return existing, nil
		}

		// A terminal failed/canceled checkout payment may be retried with a new
		// Financial Core idempotency identity. The allocation row is locked by the
		// caller, and the database also permits only one active attempt per
		// installment, so concurrent retries converge on a single new attempt.
		retryID = uuid.New()
	} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, err
	}

	firstCharges := make([]model.InstallmentCharge, 0)
	for _, charge := range charges {
		if charge.InstallmentID == firstInstallment.ID {
			firstCharges = append(firstCharges, charge)
		}
	}

	items, err := model.BuildPaymentAttemptItems(firstInstallment, firstCharges)
	if err != nil {
		return model.PaymentAttempt{}, fmt.Errorf("%s: build checkout payment attempt items: %w", op, err)
	}

	payableAmount, ok := model.PayableAmountFromItems(items)
	if !ok || payableAmount != contractRow.PayableNowAmountMinor {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf("%s: checkout payment attempt snapshot is inconsistent", op))
	}

	if amountErr := model.ValidatePositiveAmountMinor(payableAmount); amountErr != nil {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf("%s: checkout payment attempt amount: %w", op, amountErr))
	}

	providerCode := optionalTrimmedString(input.ProviderCode)

	// Financial Core v2 derives scope from the payment target. Do not require or
	// fabricate TenantID/ContextID for new checkout payment attempts.
	attempt, err := paymentAttemptQ.CreateCheckoutWithItems(
		ctx,
		contractRow.ID,
		firstInstallment.ID,
		contractRow.PayableNowAmountMinor,
		contractRow.Currency,
		model.GatewayPaymentChannel,
		checkoutPaymentIdempotencyKey(contractRow.ID, retryID),
		checkoutPaymentRequestFingerprint(input, contractRow),
		"",
		"",
		providerCode,
		uc.paymentReturnURL,
		uc.paymentReturnURL,
		uc.paymentReturnURL,
		items,
		now,
		now,
	)
	if err == nil {
		return attempt, nil
	}

	if !cerr.IsCode(err, cerr.CodeConflict) && !cerr.IsCode(err, cerr.CodeValidationFailed) {
		return model.PaymentAttempt{}, err
	}

	existing, reloadErr := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx,
		contractRow.ID,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if reloadErr != nil {
		return model.PaymentAttempt{}, err
	}

	if validateErr := validateCheckoutPaymentAttempt(existing, input, contractRow, firstInstallment, uc.paymentReturnURL); validateErr != nil {
		return model.PaymentAttempt{}, validateErr
	}

	return existing, nil
}

// ensureActiveCheckoutPaymentAttempt returns the current non-terminal checkout
// attempt. A failed or canceled attempt is replaced by exactly one new attempt
// with a new Financial Core idempotency identity. Concurrent callers converge
// on that row through the installment lock and the single-active-attempt index.
func (uc *UseCase) ensureActiveCheckoutPaymentAttempt(
	ctx context.Context,
	contractRow model.Contract,
	attempt model.PaymentAttempt,
) (model.PaymentAttempt, error) {
	if attempt.State != model.FailedPaymentAttemptState && attempt.State != model.CanceledPaymentAttemptState {
		return attempt, nil
	}

	if uc.BeforeTerminalCheckoutPaymentReplacement != nil {
		if err := uc.BeforeTerminalCheckoutPaymentReplacement(ctx); err != nil {
			return model.PaymentAttempt{}, err
		}
	}

	return uc.replaceTerminalCheckoutPaymentAttempt(ctx, contractRow, attempt)
}

func (uc *UseCase) replaceTerminalCheckoutPaymentAttempt(
	ctx context.Context,
	contractRow model.Contract,
	attempt model.PaymentAttempt,
) (model.PaymentAttempt, error) {
	const op = "contractuc.UseCase.replaceTerminalCheckoutPaymentAttempt"

	if len(attempt.Items) == 0 {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf("%s: terminal checkout attempt has no items", op))
	}

	successURL := strings.TrimSpace(optionalStringValue(attempt.SuccessReturnURL))
	failureURL := strings.TrimSpace(optionalStringValue(attempt.FailureReturnURL))
	cancelURL := strings.TrimSpace(optionalStringValue(attempt.CancelReturnURL))
	if successURL == "" || failureURL == "" || cancelURL == "" {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf("%s: terminal checkout attempt return url is incomplete", op))
	}

	var replaced model.PaymentAttempt
	now := uc.clock.Now().UTC()
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			contractQ := uc.contract.Tx(tx)
			attemptQ := uc.paymentAttempt.Tx(tx)

			if _, lockErr := contractQ.LockByID(ctx, contractRow.ID); lockErr != nil {
				return lockErr
			}

			current, currentErr := attemptQ.GetByContractIDAndPurposeWithItems(
				ctx,
				contractRow.ID,
				model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
			)
			if currentErr != nil {
				return currentErr
			}

			if current.State != model.FailedPaymentAttemptState && current.State != model.CanceledPaymentAttemptState {
				replaced = current
				return nil
			}

			created, createErr := attemptQ.CreateCheckoutWithItems(
				ctx,
				contractRow.ID,
				current.InstallmentID,
				current.AmountMinor,
				current.Currency,
				current.Channel,
				checkoutPaymentIdempotencyKey(contractRow.ID, uuid.New()),
				current.RequestFingerprint,
				optionalStringValue(current.TenantID),
				optionalStringValue(current.ContextID),
				current.ProviderCode,
				successURL,
				failureURL,
				cancelURL,
				current.Items,
				now,
				now,
			)
			if createErr != nil {
				if !cerr.IsCode(createErr, cerr.CodeConflict) && !cerr.IsCode(createErr, cerr.CodeValidationFailed) {
					return createErr
				}

				reloaded, reloadErr := attemptQ.GetByContractIDAndPurposeWithItems(
					ctx,
					contractRow.ID,
					model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
				)
				if reloadErr != nil {
					return createErr
				}

				if reloaded.State == model.FailedPaymentAttemptState || reloaded.State == model.CanceledPaymentAttemptState {
					return createErr
				}

				replaced = reloaded
				return nil
			}

			obligationID := strings.TrimSpace(optionalStringValue(current.FinancialCollectionObligationID))
			if obligationID == "" {
				replaced = created
				return nil
			}

			bound, bindErr := attemptQ.BindCollectionObligationIfAbsent(ctx, created.ID, obligationID, now)
			if bindErr != nil {
				return bindErr
			}

			replaced = bound
			return nil
		})
	})
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return replaced, nil
}

func validateCheckoutPaymentAttempt(
	attempt model.PaymentAttempt,
	input CreateContractInput,
	contractRow model.Contract,
	firstInstallment model.Installment,
	paymentReturnURL string,
) error {
	const op = "contractuc.validateCheckoutPaymentAttempt"

	if attempt.Purpose != model.CheckoutFirstInstallmentFinancialCorePaymentPurpose ||
		attempt.ContractID != contractRow.ID ||
		attempt.InstallmentID != firstInstallment.ID ||
		attempt.AmountMinor != contractRow.PayableNowAmountMinor ||
		attempt.Currency != contractRow.Currency ||
		attempt.Channel != model.GatewayPaymentChannel ||
		!isCheckoutPaymentIdempotencyKey(contractRow.ID, attempt.IdempotencyKey) ||
		attempt.RequestFingerprint != checkoutPaymentRequestFingerprint(input, contractRow) {
		return checkoutPaymentAttemptIdempotencyConflictError(fmt.Errorf("%s: checkout payment attempt does not match request", op))
	}

	if !sameOptionalString(attempt.ProviderCode, strings.TrimSpace(input.ProviderCode)) ||
		!sameOptionalString(attempt.SuccessReturnURL, paymentReturnURL) ||
		!sameOptionalString(attempt.FailureReturnURL, paymentReturnURL) ||
		!sameOptionalString(attempt.CancelReturnURL, paymentReturnURL) {
		return checkoutPaymentAttemptIdempotencyConflictError(fmt.Errorf("%s: checkout payment attempt context does not match request", op))
	}

	payable, ok := model.PayableAmountFromItems(attempt.Items)
	if !ok || payable != attempt.AmountMinor {
		return cerr.InternalError(fmt.Errorf("%s: checkout payment attempt snapshot is inconsistent", op))
	}

	return nil
}

func optionalTrimmedString(raw string) *string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}

	return &value
}

func sameOptionalString(stored *string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return stored == nil || strings.TrimSpace(*stored) == ""
	}

	return stored != nil && strings.TrimSpace(*stored) == expected
}
