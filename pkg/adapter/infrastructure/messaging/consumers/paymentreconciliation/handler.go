package paymentreconciliation

import (
	"context"
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
)

// Handler processes Financial Core installment payment reconciliation result Kafka messages.
type Handler struct {
	reconciliation *financialcorepaymentreconciliationuc.UseCase
	logger         log.Logger
}

// New creates a payment reconciliation result consumer handler.
func New(
	reconciliation *financialcorepaymentreconciliationuc.UseCase,
	logger log.Logger,
) *Handler {
	return &Handler{
		reconciliation: reconciliation,
		logger:         logger,
	}
}

// Handle decodes a Financial Core reconciliation result and applies it durably.
func (h *Handler) Handle(ctx context.Context, msg messaging.Message) error {
	const op = "paymentreconciliation.Handler.Handle"

	result, err := DecodeResult(msg.Value)
	if err != nil {
		return messaging.NonRetryable(fmt.Errorf("%s: decode financial core reconciliation result: %w", op, err))
	}

	occurredAt := result.Data.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = result.OccurredAt
	}

	input := financialcorepaymentreconciliationuc.ProcessInput{
		EventID:       result.EventID,
		EventType:     result.EventType,
		EventVersion:  result.EventVersion,
		SourceService: result.SourceService,
		CorrelationID: result.CorrelationID,
		CausationID:   result.CausationID,
		RawPayload:    msg.Value,
		Data: installmentpaymentreconciliation.ResultEventData{
			RequestEventID:            result.Data.RequestEventID,
			BNPLPaymentAttemptID:      result.Data.BNPLPaymentAttemptID,
			ReconciliationGeneration:  result.Data.ReconciliationGeneration,
			Status:                    installmentpaymentreconciliation.ResultStatus(result.Data.Status),
			FinancialPaymentIntentID:  result.Data.FinancialPaymentIntentID,
			FinancialPaymentAttemptID: result.Data.FinancialPaymentAttemptID,
			FinancialPaymentSessionID: result.Data.FinancialPaymentSessionID,
			PaymentURL:                result.Data.PaymentURL,
			ExpiresAt:                 result.Data.ExpiresAt,
			PaidAt:                    result.Data.PaidAt,
			FailureCode:               result.Data.FailureCode,
			AmountMinor:               result.Data.AmountMinor,
			Currency:                  result.Data.Currency,
			OccurredAt:                occurredAt.UTC(),
		},
	}

	output, err := h.reconciliation.Process(ctx, input)
	if err != nil {
		return classifyHandlerError(err)
	}

	h.logger.With(
		"event_id", result.EventID,
		"bnpl_payment_attempt_id", result.Data.BNPLPaymentAttemptID,
		"result", output.Result,
	).Info("processed financial core installment payment reconciliation result")

	return nil
}

func classifyHandlerError(err error) error {
	if err == nil {
		return nil
	}

	if messaging.IsNonRetryable(err) {
		return err
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var reconciliationErr *contractuc.ReconciliationError
	if errors.As(err, &reconciliationErr) {
		return messaging.NonRetryable(err)
	}

	// Only envelope/business input that cannot be durably identified is safe to
	// acknowledge directly. Conflicts and validation failures may originate from
	// a rolled-back database transaction; retry them unless the use case has
	// already converted the outcome into a committed, non-error ProcessResult.
	if cerr.IsCode(err, cerr.CodeInvalidArgument) {
		return messaging.NonRetryable(err)
	}

	return err
}
