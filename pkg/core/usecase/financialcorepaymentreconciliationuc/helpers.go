package financialcorepaymentreconciliationuc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func normalizeProcessInput(input ProcessInput) ProcessInput {
	input.EventID = strings.TrimSpace(input.EventID)
	input.EventType = strings.TrimSpace(input.EventType)
	input.EventVersion = strings.TrimSpace(input.EventVersion)
	input.SourceService = strings.TrimSpace(input.SourceService)

	return input
}

func validateMinimumTrustedEnvelope(input ProcessInput) error {
	const op = "financialcorepaymentreconciliationuc.validateMinimumTrustedEnvelope"

	if input.EventID == "" {
		return eventIDRequiredError()
	}

	if input.EventType != installmentpaymentreconciliation.ReconciliationResultEventType {
		return unsupportedEventTypeError(fmt.Errorf("%s: unsupported reconciliation result event type %q", op, input.EventType))
	}

	if input.EventVersion != installmentpaymentreconciliation.ReconciliationResultEventVersion {
		return unsupportedEventVersionError(fmt.Errorf("%s: unsupported reconciliation result event version %q", op, input.EventVersion))
	}

	if input.SourceService != installmentpaymentreconciliation.FinancialCoreSourceService {
		return unsupportedSourceServiceError(fmt.Errorf("%s: unsupported reconciliation result source service %q", op, input.SourceService))
	}

	return nil
}

func inboxSubjectForPaymentAttempt(attemptID model.PaymentAttemptID) (*string, *string) {
	subjectType := string(model.PaymentAttemptInboxSubjectType)
	subjectID := attemptID.String()

	return &subjectType, &subjectID
}

type inboxAcquire struct {
	Result model.InboxAcquireResult
	Row    model.Inbox
}

func (uc *UseCase) acquireInboxEvent(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	input ProcessInput,
	subjectType *string,
	subjectID *string,
	now time.Time,
) (inboxAcquire, error) {
	payloadFingerprint, err := uc.reconciliationResultPayloadFingerprint(input)
	if err != nil {
		return inboxAcquire{}, err
	}

	row, result, err := inboxQ.AcquireForProcessing(
		ctx,
		model.InboxEvent{
			EventID:            input.EventID,
			EventType:          input.EventType,
			EventVersion:       input.EventVersion,
			SourceService:      input.SourceService,
			CorrelationID:      input.CorrelationID,
			CausationID:        input.CausationID,
			Payload:            input.RawPayload,
			PayloadFingerprint: payloadFingerprint,
			ConsumerName:       model.FinancialCoreInstallmentPaymentReconciliationResultInboxConsumerName,
			SubjectType:        subjectType,
			SubjectID:          subjectID,
		},
		now,
	)
	if err != nil {
		return inboxAcquire{}, err
	}

	return inboxAcquire{Result: result, Row: row}, nil
}

type reconciliationResultFingerprintPayload struct {
	EventID                   string                                        `json:"event_id"`
	RequestEventID            string                                        `json:"request_event_id"`
	BNPLPaymentAttemptID      model.PaymentAttemptID                        `json:"bnpl_payment_attempt_id"`
	ReconciliationGeneration  int                                           `json:"reconciliation_generation"`
	Status                    installmentpaymentreconciliation.ResultStatus `json:"status"`
	AmountMinor               uint64                                        `json:"amount_minor"`
	Currency                  string                                        `json:"currency"`
	FinancialPaymentIntentID  *string                                       `json:"financial_payment_intent_id"`
	FinancialPaymentAttemptID *string                                       `json:"financial_payment_attempt_id"`
	FinancialPaymentSessionID *string                                       `json:"financial_payment_session_id"`
	PaymentURL                *string                                       `json:"payment_url"`
	ExpiresAt                 *string                                       `json:"expires_at"`
	PaidAt                    *string                                       `json:"paid_at"`
	FailureCode               *string                                       `json:"failure_code"`
	OccurredAt                string                                        `json:"occurred_at"`
}

func (uc *UseCase) reconciliationResultPayloadFingerprint(input ProcessInput) (string, error) {
	const op = "financialcorepaymentreconciliationuc.UseCase.reconciliationResultPayloadFingerprint"

	payload := reconciliationResultFingerprintPayload{
		EventID:                   input.EventID,
		RequestEventID:            strings.TrimSpace(input.Data.RequestEventID),
		BNPLPaymentAttemptID:      input.Data.BNPLPaymentAttemptID,
		ReconciliationGeneration:  input.Data.ReconciliationGeneration,
		Status:                    input.Data.Status,
		AmountMinor:               input.Data.AmountMinor,
		Currency:                  strings.TrimSpace(input.Data.Currency),
		FinancialPaymentIntentID:  trimOptionalString(input.Data.FinancialPaymentIntentID),
		FinancialPaymentAttemptID: trimOptionalString(input.Data.FinancialPaymentAttemptID),
		FinancialPaymentSessionID: trimOptionalString(input.Data.FinancialPaymentSessionID),
		PaymentURL:                trimOptionalString(input.Data.PaymentURL),
		ExpiresAt:                 formatOptionalUTCTime(input.Data.ExpiresAt),
		PaidAt:                    formatOptionalUTCTime(input.Data.PaidAt),
		FailureCode:               trimOptionalString(input.Data.FailureCode),
		OccurredAt:                input.Data.OccurredAt.UTC().Format(time.RFC3339Nano),
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%s: marshal reconciliation result fingerprint payload: %w", op, err)
	}

	value, err := uc.fingerprintHasher.Hash(string(raw))
	if err != nil {
		return "", fmt.Errorf("%s: hash fingerprint: %w", op, err)
	}

	if err := value.Validate(); err != nil {
		return "", fmt.Errorf("%s: validate fingerprint: %w", op, err)
	}

	return value.String(), nil
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func formatOptionalUTCTime(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func mapInboxAcquireResult(result model.InboxAcquireResult, output *ProcessOutput) bool {
	switch result {
	case model.InboxAcquireAlreadyProcessed:
		output.Result = ProcessResultAlreadyProcessed
		return true

	case model.InboxAcquireReconciliationRequired:
		output.Result = ProcessResultStillReconciliationRequired
		return true

	case model.InboxAcquirePermanentlyFailed:
		output.Result = ProcessResultPermanentlyRejected
		return true

	default:
		return false
	}
}

func (uc *UseCase) recordPayloadIdentityConflict(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	inboxRow model.Inbox,
	input ProcessInput,
	now time.Time,
	output *ProcessOutput,
) error {
	incomingFingerprint, err := uc.reconciliationResultPayloadFingerprint(input)
	if err != nil {
		return err
	}

	_, _, err = inboxQ.RecordPayloadIdentityConflict(
		ctx,
		model.InboxPayloadConflictEvent{
			InboxID:                    inboxRow.ID,
			SourceService:              input.SourceService,
			EventID:                    input.EventID,
			ExistingPayloadFingerprint: inboxRow.PayloadFingerprint,
			IncomingPayloadFingerprint: incomingFingerprint,
			IncomingPayload:            input.RawPayload,
			FailureCode:                "event_identity_payload_mismatch",
		},
		now,
	)
	if err != nil {
		return err
	}

	output.Result = ProcessResultPayloadConflictRecorded

	return nil
}

func (uc *UseCase) persistPermanentInboxFailure(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	inboxRow model.Inbox,
	failureCode string,
	failureMessage string,
	now time.Time,
	output *ProcessOutput,
) error {
	const op = "financialcorepaymentreconciliationuc.UseCase.persistPermanentInboxFailure"

	if inboxRow.Status == model.FailedInboxStatus {
		output.Result = ProcessResultPermanentlyRejected
		return nil
	}

	if inboxRow.Status != model.ReceivedInboxStatus {
		return inboxNotReceivableConflictError(fmt.Errorf("%s: inbox row is not receivable for permanent failure", op))
	}

	if _, err := inboxQ.MarkFailed(
		ctx,
		inboxRow.ID,
		failureCode,
		failureMessage,
		now,
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultPermanentlyRejected

	return nil
}

func (uc *UseCase) persistPermanentInboxFailureWithAcknowledgement(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxRow model.Inbox,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	failureCode string,
	failureMessage string,
	reason *model.PaymentReconciliationReason,
	now time.Time,
	output *ProcessOutput,
) error {
	const op = "financialcorepaymentreconciliationuc.UseCase.persistPermanentInboxFailureWithAcknowledgement"

	if inboxRow.Status == model.FailedInboxStatus {
		output.Result = ProcessResultPermanentlyRejected
		return nil
	}

	if inboxRow.Status != model.ReceivedInboxStatus {
		return inboxNotReceivableConflictError(fmt.Errorf("%s: inbox row is not receivable for permanent failure", op))
	}

	if _, err := inboxQ.MarkFailed(
		ctx,
		inboxRow.ID,
		failureCode,
		failureMessage,
		now,
		now,
	); err != nil {
		return err
	}

	if err := uc.insertReconciliationAcknowledgement(
		ctx,
		outboxQ,
		input,
		attempt,
		previousState,
		attempt.State,
		installmentpaymentreconciliation.PermanentlyRejectedAcknowledgementResult,
		attempt.FinancialPaymentIntentID,
		reason,
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultPermanentlyRejected
	return nil
}

func (uc *UseCase) persistReconciliationRequiredWithAcknowledgement(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxRow model.Inbox,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	failureCode string,
	failureMessage string,
	reason model.PaymentReconciliationReason,
	now time.Time,
	output *ProcessOutput,
) error {
	const op = "financialcorepaymentreconciliationuc.UseCase.persistReconciliationRequiredWithAcknowledgement"

	if inboxRow.Status == model.ReconciliationRequiredInboxStatus {
		output.Result = ProcessResultStillReconciliationRequired
		return nil
	}

	if inboxRow.Status != model.ReceivedInboxStatus {
		return inboxNotReceivableConflictError(fmt.Errorf("%s: inbox row is not receivable for reconciliation", op))
	}

	if _, err := inboxQ.MarkReconciliationRequired(
		ctx,
		inboxRow.ID,
		&failureCode,
		&failureMessage,
		now,
		now,
	); err != nil {
		return err
	}

	if err := uc.insertReconciliationAcknowledgement(
		ctx,
		outboxQ,
		input,
		attempt,
		previousState,
		attempt.State,
		installmentpaymentreconciliation.StillReconciliationRequiredAcknowledgementResult,
		attempt.FinancialPaymentIntentID,
		&reason,
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultStillReconciliationRequired
	return nil
}

// pendingSessionMergeClassifies how a pending reconciliation result merges with
// durable PaymentAttempt session fields before any submitted transition.
type pendingSessionMerge struct {
	IntentID   string
	SessionID  string
	PaymentURL *string
	ExpiresAt  *time.Time

	IdentityConflict bool
	Complete         bool
}

func mergePendingSession(
	attempt model.PaymentAttempt,
	data installmentpaymentreconciliation.ResultEventData,
) pendingSessionMerge {
	intentID := stringValue(data.FinancialPaymentIntentID)
	if intentID == "" && attempt.FinancialPaymentIntentID != nil {
		intentID = strings.TrimSpace(*attempt.FinancialPaymentIntentID)
	}

	sessionID := stringValue(data.FinancialPaymentSessionID)
	if sessionID == "" && attempt.FinancialPaymentSessionID != nil {
		sessionID = strings.TrimSpace(*attempt.FinancialPaymentSessionID)
	}

	if attempt.FinancialPaymentIntentID != nil &&
		strings.TrimSpace(*attempt.FinancialPaymentIntentID) != "" &&
		intentID != "" &&
		intentID != strings.TrimSpace(*attempt.FinancialPaymentIntentID) {
		return pendingSessionMerge{IdentityConflict: true}
	}

	if attempt.FinancialPaymentSessionID != nil &&
		strings.TrimSpace(*attempt.FinancialPaymentSessionID) != "" &&
		sessionID != "" &&
		sessionID != strings.TrimSpace(*attempt.FinancialPaymentSessionID) {
		return pendingSessionMerge{IdentityConflict: true}
	}

	// PaymentURL: when FC returns a non-empty URL for an otherwise matching
	// PaymentIntent/PaymentSession identity, prefer the event value. Available
	// FC contracts document the field as optional session metadata and do not
	// state whether URLs are immutable per PaymentSession; BNPL therefore
	// accepts a validated replacement rather than inventing a conflict rule.
	// ValidateActionablePaymentSession still rejects unsafe URLs before submit.
	var paymentURL *string
	if data.PaymentURL != nil && strings.TrimSpace(*data.PaymentURL) != "" {
		value := strings.TrimSpace(*data.PaymentURL)
		paymentURL = &value
	} else if attempt.PaymentURL != nil && strings.TrimSpace(*attempt.PaymentURL) != "" {
		value := strings.TrimSpace(*attempt.PaymentURL)
		paymentURL = &value
	}

	var expiresAt *time.Time
	if data.ExpiresAt != nil {
		value := data.ExpiresAt.UTC()
		expiresAt = &value
	} else if attempt.FinancialPaymentExpiresAt != nil {
		value := attempt.FinancialPaymentExpiresAt.UTC()
		expiresAt = &value
	}

	complete := intentID != "" && sessionID != "" && paymentURL != nil && expiresAt != nil
	return pendingSessionMerge{
		IntentID:   intentID,
		SessionID:  sessionID,
		PaymentURL: paymentURL,
		ExpiresAt:  expiresAt,
		Complete:   complete,
	}
}

// requestedPaymentExpiresAt reconstructs the original BNPL-requested CreateCheckoutPayment
// expires_at for soft late-expiry warnings. Returns ok=false when the deadline cannot be
// reconstructed safely for the attempt purpose (do not invent a fabricated expected value).
func requestedPaymentExpiresAt(
	attempt model.PaymentAttempt,
	contractExpiresAt *time.Time,
) (time.Time, bool) {
	switch attempt.Purpose {
	case model.InstallmentRepaymentFinancialCorePaymentPurpose:
		if attempt.CreatedAt.IsZero() {
			return time.Time{}, false
		}

		return attempt.CreatedAt.UTC().Add(model.InstallmentRepaymentPaymentSessionTTL), true

	case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose:
		if contractExpiresAt == nil || contractExpiresAt.IsZero() {
			return time.Time{}, false
		}

		return contractExpiresAt.UTC(), true

	default:
		return time.Time{}, false
	}
}

func (uc *UseCase) requestedExpiresAtForWarning(
	ctx context.Context,
	contractQ repo.ContractTxQueryer,
	attempt model.PaymentAttempt,
) (time.Time, bool) {
	switch attempt.Purpose {
	case model.InstallmentRepaymentFinancialCorePaymentPurpose:
		return requestedPaymentExpiresAt(attempt, nil)

	case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose:
		if attempt.ContractID.IsZero() {
			return time.Time{}, false
		}

		contractRow, err := contractQ.GetByID(ctx, attempt.ContractID)
		if err != nil {
			return time.Time{}, false
		}

		expiresAt := contractRow.ExpiresAt
		return requestedPaymentExpiresAt(attempt, &expiresAt)

	default:
		return time.Time{}, false
	}
}

func pendingResultEconomicsConflict(
	input ProcessInput,
	attempt model.PaymentAttempt,
) error {
	if input.Data.AmountMinor != attempt.AmountMinor {
		return errors.New("pending reconciliation amount conflicts with local payment attempt")
	}

	if model.Currency(strings.TrimSpace(input.Data.Currency)) != attempt.Currency {
		return errors.New("pending reconciliation currency conflicts with local payment attempt")
	}

	return nil
}

func validateSucceededResultAgainstAttempt(
	input ProcessInput,
	attempt model.PaymentAttempt,
) error {
	intentID := stringValue(input.Data.FinancialPaymentIntentID)
	if intentID == "" {
		return errors.New("succeeded reconciliation result requires financial payment intent id")
	}

	if attempt.FinancialPaymentIntentID == nil || strings.TrimSpace(*attempt.FinancialPaymentIntentID) == "" {
		return errors.New("succeeded local payment attempt has no financial payment intent id")
	}

	if strings.TrimSpace(*attempt.FinancialPaymentIntentID) != intentID {
		return errors.New("succeeded reconciliation payment intent conflicts with local attempt")
	}

	if input.Data.AmountMinor != attempt.AmountMinor {
		return errors.New("succeeded reconciliation amount conflicts with local attempt")
	}

	if model.Currency(input.Data.Currency) != attempt.Currency {
		return errors.New("succeeded reconciliation currency conflicts with local attempt")
	}

	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func stringPtr(value string) *string {
	return &value
}

func validateReconciliationRequestIdentity(
	input ProcessInput,
	attempt model.PaymentAttempt,
) error {
	expected := installmentpaymentreconciliation.RequestEventID(
		attempt.ID,
		input.Data.ReconciliationGeneration,
	)
	if strings.TrimSpace(input.Data.RequestEventID) != expected {
		return errors.New("reconciliation request event id does not match local attempt")
	}

	return nil
}

func (uc *UseCase) finishStaleSuperseded(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	now time.Time,
	output *ProcessOutput,
) error {
	return uc.finishReconciliationAcknowledgement(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		attempt,
		previousState,
		attempt.State,
		installmentpaymentreconciliation.AlreadyProcessedAcknowledgementResult,
		attempt.FinancialPaymentIntentID,
		attempt.ReconciliationReason,
		now,
		output,
		ProcessResultStaleSuperseded,
	)
}

func (uc *UseCase) nextReconciliationAt(attemptCount int) time.Time {
	delay, err := installmentpaymentreconciliation.RetryDelay(uc.retryBaseDelay, uc.retryMaxDelay, attemptCount)
	if err != nil {
		delay = uc.retryMaxDelay
	}

	return uc.clock.Add(uc.clock.Now(), delay)
}

func (uc *UseCase) finishAlreadyProcessed(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	now time.Time,
	output *ProcessOutput,
) error {
	return uc.finishReconciliationAcknowledgement(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		attempt,
		previousState,
		attempt.State,
		installmentpaymentreconciliation.AlreadyProcessedAcknowledgementResult,
		attempt.FinancialPaymentIntentID,
		attempt.ReconciliationReason,
		now,
		output,
		ProcessResultAlreadyProcessed,
	)
}

func (uc *UseCase) finishReconciliationAcknowledgement(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	currentState model.PaymentAttemptState,
	result installmentpaymentreconciliation.AcknowledgementResult,
	financialPaymentIntentID *string,
	reason *model.PaymentReconciliationReason,
	now time.Time,
	output *ProcessOutput,
	processResult ProcessResult,
) error {
	if _, err := inboxQ.MarkProcessed(ctx, inboxID, now, now); err != nil {
		return err
	}

	if err := uc.insertReconciliationAcknowledgement(
		ctx,
		outboxQ,
		input,
		attempt,
		previousState,
		currentState,
		result,
		financialPaymentIntentID,
		reason,
		now,
	); err != nil {
		return err
	}

	output.Result = processResult

	return nil
}

func (uc *UseCase) insertReconciliationAcknowledgement(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	currentState model.PaymentAttemptState,
	result installmentpaymentreconciliation.AcknowledgementResult,
	financialPaymentIntentID *string,
	reason *model.PaymentReconciliationReason,
	now time.Time,
) error {
	const op = "financialcorepaymentreconciliationuc.UseCase.insertReconciliationAcknowledgement"

	event := installmentpaymentreconciliation.NewAcknowledgementEvent(
		uc.idGenerator.Generate(),
		now,
		installmentpaymentreconciliation.AcknowledgementEventData{
			RequestEventID:             input.Data.RequestEventID,
			FinancialCoreResultEventID: input.EventID,
			BNPLPaymentAttemptID:       attempt.ID,
			ReconciliationGeneration:   input.Data.ReconciliationGeneration,
			PreviousState:              previousState,
			CurrentState:               currentState,
			Result:                     result,
			FinancialPaymentIntentID:   financialPaymentIntentID,
			Reason:                     reason,
			BNPLProcessedAt:            now,
		},
	)

	if err := installmentpaymentreconciliation.ValidateAcknowledgementEvent(event); err != nil {
		return fmt.Errorf("%s: validate reconciliation acknowledgement event: %w", op, err)
	}

	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal reconciliation acknowledgement payload: %w", op, err)
	}

	aggregateType := "payment_attempt"
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.reconciliationAcknowledgementsDestination,
		MessageKey:    attempt.ID.String(),
		EventType:     installmentpaymentreconciliation.ReconciliationAcknowledgedEventType,
		AggregateType: &aggregateType,
		Payload:       raw,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert reconciliation acknowledgement outbox message: %w", op, err)
	}

	return nil
}
