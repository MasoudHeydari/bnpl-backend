package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	// PaymentStatusRecoveryEventType identifies a BNPL-initiated GetPaymentStatus
	// recovery wake-up. It is not a Financial Core Kafka contract.
	PaymentStatusRecoveryEventType = "bnpl.payment_status_recovery.v1"

	// PaymentStatusRecoverySourceService identifies BNPL as the recovery actor.
	PaymentStatusRecoverySourceService = financialCoreBNPLSourceService

	paymentStatusRecoveryEventVersion = "1"
)

// ErrPaymentStillInFlight means Financial Core reported REQUIRES_ACTION or
// PROCESSING. That is not a payment failure; callers must retry later.
var ErrPaymentStillInFlight = errors.New("financial core payment is still in flight")

// RecoverByGetPaymentStatusInput asks BNPL to poll Financial Core for the
// authoritative payment outcome of a stored payment intent.
type RecoverByGetPaymentStatusInput struct {
	PaymentAttemptID model.PaymentAttemptID
	// Generation scopes the durable recovery inbox identity so retries after a
	// still-in-flight response do not collide with a prior non-terminal wake-up.
	Generation int
	// CorrelationID is optional operational correlation for the recovery audit.
	CorrelationID *string
}

// RecoverByGetPaymentStatusOutput reports how recovery completed.
type RecoverByGetPaymentStatusOutput struct {
	Result        ProcessResult
	PaymentStatus model.FinancialPaymentResultStatus
	StillInFlight bool
}

// RecoverByGetPaymentStatus polls GetPaymentStatus and applies a terminal
// authoritative outcome through the normal payment-result path.
//
// Preconditions: the payment attempt must already store FinancialPaymentIntentID.
// Missing intent cannot be recovered here — callers must same-key replay
// CreateCheckoutPayment when session creation is still open, or escalate to
// operational review when it is not.
//
// PROCESSING / REQUIRES_ACTION return ErrPaymentStillInFlight without inventing
// failure or success.
func (uc *UseCase) RecoverByGetPaymentStatus(
	ctx context.Context,
	input RecoverByGetPaymentStatusInput,
) (RecoverByGetPaymentStatusOutput, error) {
	const op = "financialcorepaymentresultuc.UseCase.RecoverByGetPaymentStatus"

	if input.PaymentAttemptID.IsZero() {
		return RecoverByGetPaymentStatusOutput{}, fmt.Errorf("%s: payment attempt id is required", op)
	}

	generation := input.Generation
	if generation <= 0 {
		generation = 1
	}

	var attempt model.PaymentAttempt
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			var loadErr error
			attempt, loadErr = paymentAttemptQ.LockByID(ctx, input.PaymentAttemptID)
			return loadErr
		})
	})
	if err != nil {
		return RecoverByGetPaymentStatusOutput{}, err
	}

	paymentIntentID := ""
	if attempt.FinancialPaymentIntentID != nil {
		paymentIntentID = strings.TrimSpace(*attempt.FinancialPaymentIntentID)
	}

	if paymentIntentID == "" {
		return RecoverByGetPaymentStatusOutput{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt has no financial payment intent id", op),
			map[string]any{"payment_attempt_id": attempt.ID},
		)
	}

	now := uc.clock.Now().UTC()
	eventID := paymentStatusRecoveryEventID(attempt.ID, generation)
	rawPayload, err := json.Marshal(map[string]any{
		"event_id":            eventID,
		"event_type":          PaymentStatusRecoveryEventType,
		"payment_attempt_id":  attempt.ID.String(),
		"payment_intent_id":   paymentIntentID,
		"purpose":             string(attempt.Purpose),
		"recovery_generation": generation,
		"recovered_at":        now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return RecoverByGetPaymentStatusOutput{}, fmt.Errorf("%s: marshal recovery payload: %w", op, err)
	}

	processInput := ProcessInput{
		EventID:         eventID,
		EventType:       PaymentStatusRecoveryEventType,
		EventVersion:    paymentStatusRecoveryEventVersion,
		SourceService:   PaymentStatusRecoverySourceService,
		CorrelationID:   input.CorrelationID,
		RawPayload:      rawPayload,
		PaymentPurpose:  attempt.Purpose,
		ContractID:      attempt.ContractID,
		InstallmentID:   attempt.InstallmentID,
		PaymentIntentID: paymentIntentID,
		// Placeholder only — resolveAuthoritative* overwrites from GetPaymentStatus
		// before any money-state mutation.
		PaymentStatus:   model.FailedFinancialPaymentResultStatus,
		EventOccurredAt: now,
	}

	switch attempt.Purpose {
	case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose:
		processInput.SourceType = financialCoreBNPLContractType
		processInput.SourceID = attempt.ContractID.String()

	case model.InstallmentRepaymentFinancialCorePaymentPurpose:
		processInput.SourceType = financialCorePaymentAttemptSourceType
		processInput.SourceID = attempt.ID.String()

	default:
		return RecoverByGetPaymentStatusOutput{}, cerr.Conflict(
			fmt.Errorf("%s: unsupported payment purpose %q", op, attempt.Purpose),
			map[string]any{"payment_attempt_id": attempt.ID},
		)
	}

	output, err := uc.Process(ctx, processInput)
	if err != nil {
		if isPaymentStillInFlightError(err) {
			return RecoverByGetPaymentStatusOutput{StillInFlight: true}, fmt.Errorf("%w: %w", ErrPaymentStillInFlight, err)
		}

		return RecoverByGetPaymentStatusOutput{}, err
	}

	return RecoverByGetPaymentStatusOutput{
		Result:        output.Result,
		PaymentStatus: processInput.PaymentStatus,
	}, nil
}

func paymentStatusRecoveryEventID(attemptID model.PaymentAttemptID, generation int) string {
	return fmt.Sprintf("bnpl.payment_status_recovery:%s:%d", attemptID, generation)
}

func isPaymentStillInFlightError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrPaymentStillInFlight) {
		return true
	}

	msg := err.Error()
	return strings.Contains(msg, "still "+string(extcheckoutpayment.RequiresActionPaymentStatus)) ||
		strings.Contains(msg, "still "+string(extcheckoutpayment.ProcessingPaymentStatus))
}
