package paymentsession

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentsessionuc"
)

// Handler processes Financial Core installment payment-session-created Kafka messages.
type Handler struct {
	paymentSession *financialcorepaymentsessionuc.UseCase
	logger         log.Logger
}

// New creates a payment-session consumer handler.
func New(
	paymentSession *financialcorepaymentsessionuc.UseCase,
	logger log.Logger,
) *Handler {
	return &Handler{
		paymentSession: paymentSession,
		logger:         logger,
	}
}

// Handle decodes a Financial Core payment-session-created event and applies it durably.
func (h *Handler) Handle(ctx context.Context, msg messaging.Message) error {
	const op = "paymentsession.Handler.Handle"

	sessionCreated, err := DecodeInstallmentPaymentSessionCreated(msg.Value)
	if err != nil {
		return messaging.NonRetryable(fmt.Errorf("%s: decode financial core payment session created: %w", op, err))
	}

	data := sessionCreated.Data
	occurredAt := data.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = sessionCreated.OccurredAt
	}

	input := financialcorepaymentsessionuc.ProcessInput{
		EventID:                   sessionCreated.EventID,
		EventType:                 sessionCreated.EventType,
		EventVersion:              sessionCreated.EventVersion,
		SourceService:             sessionCreated.SourceService,
		CorrelationID:             sessionCreated.CorrelationID,
		CausationID:               sessionCreated.CausationID,
		RawPayload:                msg.Value,
		BNPLPaymentAttemptID:      data.BNPLPaymentAttemptID,
		FinancialPaymentIntentID:  data.FinancialPaymentIntentID,
		FinancialPaymentAttemptID: data.FinancialPaymentAttemptID,
		FinancialPaymentSessionID: data.FinancialPaymentSessionID,
		PaymentURL:                data.PaymentURL,
		ExpiresAt:                 data.ExpiresAt.UTC(),
		AmountMinor:               data.AmountMinor,
		Currency:                  model.Currency(data.Currency),
		EventOccurredAt:           occurredAt.UTC(),
	}

	output, err := h.paymentSession.Process(ctx, input)
	if err != nil {
		return classifyHandlerError(err)
	}

	h.logger.With(
		"event_id", sessionCreated.EventID,
		"bnpl_payment_attempt_id", data.BNPLPaymentAttemptID,
		"result", output.Result,
	).Info("processed financial core installment payment session created")

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
