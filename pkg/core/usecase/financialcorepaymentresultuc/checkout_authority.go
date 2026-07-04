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

const (
	financialCoreBNPLSourceService = "bnpl-core"
	financialCoreBNPLContractType  = "bnpl_contract"
)

// resolveAuthoritativeCheckoutResult converts an inbound notification into the
// latest authoritative Financial Core state before BNPL mutates contract or
// installment state. The inbound event is a wake-up/correlation signal only.
func (uc *UseCase) resolveAuthoritativeCheckoutResult(
	ctx context.Context,
	input ProcessInput,
) (ProcessInput, error) {
	const op = "financialcorepaymentresultuc.UseCase.resolveAuthoritativeCheckoutResult"

	paymentIntentID := strings.TrimSpace(input.PaymentIntentID)
	if paymentIntentID == "" {
		return ProcessInput{}, cerr.InvalidArgument(errors.New("financial payment intent id is required for authoritative checkout lookup"))
	}

	var attempt model.PaymentAttempt
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var loadErr error
		paymentAttemptQ := uc.paymentAttempt.Conn(conn)
		// TODO(fc-early-payment-result): A Financial Core payment-result event
		// may arrive before BNPL durably saves the PaymentIntentID returned by
		// CreateCheckoutPayment. Cover with an E2E that proves temporary
		// GetByFinancialPaymentIntentID failure does not permanently lose the
		// event and that retry/ack semantics eventually apply the authoritative
		// result. Do not change this consumer path merely to satisfy that test.
		attempt, loadErr = paymentAttemptQ.GetByFinancialPaymentIntentID(ctx, paymentIntentID)
		return loadErr
	})
	if err != nil {
		return ProcessInput{}, err
	}

	if attempt.Purpose != model.CheckoutFirstInstallmentFinancialCorePaymentPurpose {
		return ProcessInput{}, cerr.Conflict(
			fmt.Errorf("%s: payment intent is not a checkout-first-installment attempt", op),
			map[string]any{"payment_intent_id": paymentIntentID},
		)
	}

	// The stored PaymentAttempt is the authoritative BNPL correlation for
	// checkout. A Kafka event may omit contract_id; leaving it empty skips the
	// customer -> contract -> installment settlement lock in Process.
	if attempt.ContractID.IsZero() {
		return ProcessInput{}, cerr.Conflict(
			fmt.Errorf("%s: checkout payment attempt has no contract", op),
		)
	}
	if !input.ContractID.IsZero() && input.ContractID != attempt.ContractID {
		return ProcessInput{}, cerr.Conflict(
			fmt.Errorf("%s: inbound contract conflicts with the payment attempt", op),
		)
	}
	if input.SourceType != "" && !strings.EqualFold(strings.TrimSpace(input.SourceType), financialCoreBNPLContractType) {
		return ProcessInput{}, cerr.Conflict(
			fmt.Errorf("%s: invalid checkout payment source type", op),
		)
	}
	if input.SourceID != "" && strings.TrimSpace(input.SourceID) != attempt.ContractID.String() {
		return ProcessInput{}, cerr.Conflict(
			fmt.Errorf("%s: inbound payment source conflicts with the payment attempt contract", op),
		)
	}

	sourceVersion := int64(1)
	status, err := uc.checkoutPaymentClient.GetPaymentStatus(ctx, extcheckoutpayment.GetPaymentStatusInput{
		SourceType:      financialCoreBNPLContractType,
		SourceID:        attempt.ContractID.String(),
		SourceVersion:   &sourceVersion,
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

	// Use the persisted relation for checkout payment-result serialization.
	// Do not trust the optional contract_id supplied by a Kafka producer.
	input.ContractID = attempt.ContractID

	switch status.Status {
	case extcheckoutpayment.RequiresActionPaymentStatus, extcheckoutpayment.ProcessingPaymentStatus:
		return ProcessInput{}, fmt.Errorf("%s: Financial Core checkout payment is still %s", op, status.Status)

	case extcheckoutpayment.SucceededPaymentStatus:
		if status.AmountMinor <= 0 {
			return ProcessInput{}, cerr.InternalError(fmt.Errorf("%s: successful Financial Core payment amount is invalid", op))
		}

		if status.CompletedAt.IsZero() {
			return ProcessInput{}, cerr.InternalError(fmt.Errorf("%s: successful Financial Core payment completed_at is missing", op))
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
		return ProcessInput{}, fmt.Errorf("%s: unsupported authoritative checkout payment status %q", op, status.Status)
	}

	return input, nil
}
