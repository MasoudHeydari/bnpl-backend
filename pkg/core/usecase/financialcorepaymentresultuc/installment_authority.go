package financialcorepaymentresultuc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// resolveAuthoritativeInstallmentResult treats the inbound payment-result event
// as a wake-up signal and reloads the current monetary result from Financial Core.
func (uc *UseCase) resolveAuthoritativeInstallmentResult(
	ctx context.Context,
	input ProcessInput,
) (ProcessInput, error) {
	const op = "financialcorepaymentresultuc.UseCase.resolveAuthoritativeInstallmentResult"

	paymentIntentID := strings.TrimSpace(input.PaymentIntentID)
	if paymentIntentID == "" {
		return ProcessInput{}, cerr.InvalidArgument(errors.New("financial payment intent id is required for authoritative installment lookup"))
	}

	var attempt model.PaymentAttempt
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var loadErr error
		paymentAttemptQ := uc.paymentAttempt.Conn(conn)
		attempt, loadErr = paymentAttemptQ.GetByFinancialPaymentIntentID(ctx, paymentIntentID)
		return loadErr
	})
	if err != nil {
		return ProcessInput{}, err
	}

	if attempt.Purpose != model.InstallmentRepaymentFinancialCorePaymentPurpose {
		return ProcessInput{}, cerr.Conflict(
			fmt.Errorf("%s: payment intent is not an installment-repayment attempt", op),
			map[string]any{"payment_intent_id": paymentIntentID},
		)
	}

	if attempt.TenantID == nil || strings.TrimSpace(*attempt.TenantID) == "" ||
		attempt.ContextID == nil || strings.TrimSpace(*attempt.ContextID) == "" {
		return ProcessInput{}, cerr.InternalError(fmt.Errorf("%s: persisted installment financial scope is incomplete", op))
	}

	status, err := uc.checkoutPaymentClient.GetPaymentStatus(ctx, extcheckoutpayment.GetPaymentStatusInput{
		TenantID:        strings.TrimSpace(*attempt.TenantID),
		ContextID:       strings.TrimSpace(*attempt.ContextID),
		SourceService:   financialCoreBNPLSourceService,
		SourceType:      financialCorePaymentAttemptSourceType,
		SourceID:        attempt.ID.String(),
		SourceVersion:   1,
		PaymentIntentID: paymentIntentID,
	})
	if err != nil {
		return ProcessInput{}, err
	}

	if strings.TrimSpace(status.PaymentIntentID) != paymentIntentID {
		return ProcessInput{}, cerr.Conflict(
			fmt.Errorf("%s: Financial Core returned a different payment intent id", op),
			map[string]any{"payment_intent_id": paymentIntentID},
		)
	}

	input.ContractID = attempt.ContractID
	input.InstallmentID = attempt.InstallmentID
	input.SourceType = financialCorePaymentAttemptSourceType
	input.SourceID = attempt.ID.String()
	input.PaymentPurpose = model.InstallmentRepaymentFinancialCorePaymentPurpose

	switch status.Status {
	case extcheckoutpayment.RequiresActionPaymentStatus, extcheckoutpayment.ProcessingPaymentStatus:
		return ProcessInput{}, fmt.Errorf("%s: Financial Core installment payment is still %s", op, status.Status)

	case extcheckoutpayment.SucceededPaymentStatus:
		if status.AmountMinor <= 0 || status.CompletedAt.IsZero() {
			return ProcessInput{}, cerr.InternalError(fmt.Errorf("%s: successful Financial Core installment result is incomplete", op))
		}

		input.PaymentStatus = model.SucceededFinancialPaymentResultStatus
		input.PaidAmountMinor = uint64(status.AmountMinor)
		input.Currency = model.Currency(strings.TrimSpace(status.Currency))
		input.PaidAt = status.CompletedAt.UTC()

	case extcheckoutpayment.FailedPaymentStatus, extcheckoutpayment.ExpiredPaymentStatus:
		input.PaymentStatus = model.FailedFinancialPaymentResultStatus
		if status.Status == extcheckoutpayment.ExpiredPaymentStatus {
			failureCode := "expired"
			input.FailureCode = &failureCode
		}

	case extcheckoutpayment.CanceledPaymentStatus:
		input.PaymentStatus = model.CanceledFinancialPaymentResultStatus

	default:
		return ProcessInput{}, fmt.Errorf("%s: unsupported authoritative installment payment status %q", op, status.Status)
	}

	return input, nil
}
