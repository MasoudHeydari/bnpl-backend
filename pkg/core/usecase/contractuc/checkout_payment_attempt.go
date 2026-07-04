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
	attempt, err := paymentAttemptQ.CreateCheckoutWithItems(
		ctx,
		contractRow.ID,
		firstInstallment.ID,
		contractRow.PayableNowAmountMinor,
		contractRow.Currency,
		model.GatewayPaymentChannel,
		checkoutPaymentIdempotencyKey(contractRow.ID, retryID),
		checkoutPaymentRequestFingerprint(input, contractRow),
		input.TenantID.String(),
		input.ContextID.String(),
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

	if !sameOptionalString(attempt.TenantID, input.TenantID.String()) ||
		!sameOptionalString(attempt.ContextID, input.ContextID.String()) ||
		!sameOptionalString(attempt.ProviderCode, strings.TrimSpace(input.ProviderCode)) ||
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
