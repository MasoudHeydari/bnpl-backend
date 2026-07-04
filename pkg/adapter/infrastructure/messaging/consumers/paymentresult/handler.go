package paymentresult

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// Handler processes Financial Core payment result Kafka messages.
type Handler struct {
	paymentResult *financialcorepaymentresultuc.UseCase
	logger        log.Logger
}

// New creates a payment result consumer handler.
func New(
	paymentResult *financialcorepaymentresultuc.UseCase,
	logger log.Logger,
) *Handler {
	return &Handler{
		paymentResult: paymentResult,
		logger:        logger,
	}
}

// Handle decodes a Financial Core payment result and applies it durably.
func (h *Handler) Handle(ctx context.Context, msg messaging.Message) error {
	const op = "paymentresult.Handler.Handle"

	paymentResult, err := DecodePaymentResult(msg.Value)
	if err != nil {
		return messaging.NonRetryable(fmt.Errorf("%s: decode financial core payment result: %w", op, err))
	}

	rawPaymentStatus := NormalizePaymentStatus(paymentResult.Status)
	paymentStatus, statusErr := MapPaymentResultStatus(rawPaymentStatus)
	if statusErr != nil {
		paymentStatus = ""
	}

	input := financialcorepaymentresultuc.ProcessInput{
		EventID:          paymentResult.EventID,
		EventType:        paymentResult.EventType,
		EventVersion:     paymentResult.EventVersion,
		SourceService:    paymentResult.SourceService,
		CorrelationID:    paymentResult.CorrelationID,
		CausationID:      paymentResult.CausationID,
		RawPayload:       msg.Value,
		PaymentPurpose:   paymentResult.PaymentPurpose,
		InvoiceID:        paymentResult.InvoiceID,
		PaymentIntentID:  paymentResult.PaymentIntentID,
		PaymentAttemptID: paymentResult.PaymentAttemptID,
		PaymentSessionID: paymentResult.PaymentSessionID,
		PaymentStatus:    paymentStatus,
		RawPaymentStatus: rawPaymentStatus,
		PaidAmountMinor:  paymentResult.PaidAmountMinor,
		Currency:         model.Currency(paymentResult.Currency),
		PaidAt:           paymentResult.PaidAt.UTC(),
		EventOccurredAt:  paymentResult.OccurredAt.UTC(),
		SourceType:       paymentResult.SourceType,
		SourceID:         paymentResult.SourceID,
		FailureCode:      paymentResult.FailureCode,
	}

	if paymentResult.ContractID != nil {
		input.ContractID = *paymentResult.ContractID
	}

	if paymentResult.InstallmentID != nil {
		input.InstallmentID = *paymentResult.InstallmentID
	}

	output, err := h.paymentResult.Process(ctx, input)
	if err != nil {
		return classifyHandlerError(err)
	}

	h.logger.With(
		"event_id", paymentResult.EventID,
		"payment_purpose", input.PaymentPurpose,
		"result", output.Result,
	).Info("processed financial core payment result")

	return nil
}

func classifyHandlerError(err error) error {
	if err == nil {
		return nil
	}

	if cerr.IsCode(err, cerr.CodeInvalidArgument) {
		return messaging.NonRetryable(err)
	}

	return err
}
