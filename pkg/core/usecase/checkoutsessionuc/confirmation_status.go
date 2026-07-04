package checkoutsessionuc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

// Customer-facing confirmation phase projected for Hosted Checkout polling.
// These are not database statuses.
const (
	ConfirmationStatusNotStarted            = "not_started"
	ConfirmationStatusProcessing            = "processing"
	ConfirmationStatusPaymentActionRequired = "payment_action_required"
	ConfirmationStatusPaymentProcessing     = "payment_processing"
	ConfirmationStatusReconfirmRequired     = "reconfirm_required"
	ConfirmationStatusRetryAvailable        = "retry_available"
	ConfirmationStatusCompleted             = "completed"
	ConfirmationStatusNeedsAttention        = "needs_attention"
	ConfirmationStatusCanceled              = "canceled"
	ConfirmationStatusExpired               = "expired"
	ConfirmationStatusUnavailable           = "unavailable"
)

// Customer-facing next actions for Hosted Checkout polling.
const (
	ConfirmationNextActionNone              = "none"
	ConfirmationNextActionWait              = "wait"
	ConfirmationNextActionResumePayment     = "resume_payment"
	ConfirmationNextActionWaitPaymentResult = "wait_payment_result"
	ConfirmationNextActionRetryPayment      = "retry_payment"
	ConfirmationNextActionReconfirm         = "reconfirm"
	ConfirmationNextActionShowSuccess       = "show_success"
	ConfirmationNextActionShowPendingReview = "show_pending_review"
	ConfirmationNextActionShowCanceled      = "show_canceled"
	ConfirmationNextActionShowExpired       = "show_expired"
	ConfirmationNextActionShowUnavailable   = "show_unavailable"
	ConfirmationNextActionStartConfirm      = "start_confirm"
)

const defaultConfirmationStatusPollSeconds = 3

// paymentContinuationClass distinguishes why a payment session cannot (or can)
// be presented as a customer continuation target.
type paymentContinuationClass int

const (
	paymentContinuationUsable paymentContinuationClass = iota
	paymentContinuationIncomplete
	paymentContinuationMismatch
	paymentContinuationExpired
	paymentContinuationIneligible
)

// GetExternalConfirmationStatusInput carries the hosted confirmation-status poll.
type GetExternalConfirmationStatusInput struct {
	Token model.CheckoutSessionToken
}

func (i GetExternalConfirmationStatusInput) validate() error {
	return validateCheckoutSessionToken(i.Token)
}

// GetExternalConfirmationStatusOutput is the privacy-safe confirmation projection.
//
// It never exposes Contract IDs, Saga IDs, ConfirmationAttempt IDs, Financial Core
// identifiers, payment redirect URLs, customer ids, phones, or raw FC errors.
// OTP countdown fields are informational; Backend enforces issuance independently.
type GetExternalConfirmationStatusOutput struct {
	Token              model.CheckoutSessionToken
	Status             model.CheckoutSessionStatus
	ConfirmationStatus string
	NextAction         string
	RetryAfterSeconds  *int
	OTPNextIssueAt     *time.Time
	OTPAvailability    string
}

// GetExternalConfirmationStatus projects durable checkout confirmation state for
// Hosted Checkout polling after an inconclusive Confirm.
//
// Strictly read-only: no Saga execution, no Decision/Contract/PaymentAttempt
// writes, no Financial Core calls, no Outbox publish, and no OTP.
func (uc *UseCase) GetExternalConfirmationStatus(
	ctx context.Context,
	input GetExternalConfirmationStatusInput,
) (GetExternalConfirmationStatusOutput, error) {
	const op = "checkoutsessionuc.UseCase.GetExternalConfirmationStatus"

	if err := input.validate(); err != nil {
		return GetExternalConfirmationStatusOutput{}, err
	}

	now := uc.clock.Now().UTC()
	var (
		session model.CheckoutSession
		output  GetExternalConfirmationStatusOutput
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sessionQ := uc.checkoutSession.Conn(conn)
		found, err := sessionQ.GetByPublicToken(ctx, input.Token)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return checkoutSessionNotFoundError(err)
			}

			return err
		}

		if found.Origin != model.ExternalPartnerCheckoutSessionOrigin {
			return checkoutSessionNotFoundError(
				fmt.Errorf("%s: checkout session origin is %q", op, found.Origin),
			)
		}

		session = found
		return nil
	})
	if err != nil {
		return GetExternalConfirmationStatusOutput{}, err
	}

	status := session.Status
	if status == model.OpenCheckoutSessionStatus && session.ContractID == nil &&
		session.IsExpiredAt(now) {
		status = model.ExpiredCheckoutSessionStatus
	}

	projection, projectErr := uc.projectConfirmationStatus(ctx, session, status, now)
	if projectErr != nil {
		return GetExternalConfirmationStatusOutput{}, projectErr
	}

	// Align OTP availability with POST /otp business blockers (manual_review,
	// reconciliation, expired/ineligible contract, ineligible PA). Prefer the
	// shared evaluateOTPIssuanceBlockersOutsideTx path over confirmation_status
	// mapping alone so status cannot show awaiting_entry when POST would reject.
	businessBlocked, blockErr := uc.evaluateOTPIssuanceBlockersOutsideTx(ctx, session, status, now)
	if blockErr != nil {
		return GetExternalConfirmationStatusOutput{}, blockErr
	}

	if !businessBlocked {
		businessBlocked = otpIssuanceBlockedByConfirmation(projection.confirmationStatus, session)
	}

	otpOut, otpErr := uc.otp.Availability(ctx, otp.AvailabilityInput{
		Purpose:         model.OTPPurposeHostedCheckoutConfirm,
		SubjectKey:      session.ID.String(),
		ContextVersion:  strconv.Itoa(session.EvaluationGeneration),
		Terminal:        isTerminalCheckoutStatus(status),
		BusinessBlocked: businessBlocked,
	})
	if otpErr != nil {
		return GetExternalConfirmationStatusOutput{}, otpErr
	}

	output = GetExternalConfirmationStatusOutput{
		Token:              session.PublicToken,
		Status:             status,
		ConfirmationStatus: projection.confirmationStatus,
		NextAction:         projection.nextAction,
		RetryAfterSeconds:  projection.retryAfterSeconds,
		OTPNextIssueAt:     otpOut.OTPNextIssueAt,
		OTPAvailability:    otpOut.Availability,
	}

	return output, nil
}

type confirmationStatusProjection struct {
	confirmationStatus string
	nextAction         string
	retryAfterSeconds  *int
}

func (uc *UseCase) projectConfirmationStatus(
	ctx context.Context,
	session model.CheckoutSession,
	status model.CheckoutSessionStatus,
	now time.Time,
) (confirmationStatusProjection, error) {
	switch status {
	case model.CompletedCheckoutSessionStatus:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusCompleted,
			nextAction:         ConfirmationNextActionShowSuccess,
		}, nil

	case model.CanceledCheckoutSessionStatus:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusCanceled,
			nextAction:         ConfirmationNextActionShowCanceled,
		}, nil

	case model.ExpiredCheckoutSessionStatus:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusExpired,
			nextAction:         ConfirmationNextActionShowExpired,
		}, nil

	case model.UnavailableCheckoutSessionStatus:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusUnavailable,
			nextAction:         ConfirmationNextActionShowUnavailable,
		}, nil
	}

	sagaRow, hasSaga, sagaErr := uc.lookupConfirmationSaga(ctx, session)
	if sagaErr != nil {
		return confirmationStatusProjection{}, sagaErr
	}

	// ConfirmationAttemptID is written atomically with Saga creation. A missing
	// Saga under a nonzero attempt id is corruption — fail closed to attention
	// rather than inviting OTP / FC continuation.
	if session.ConfirmationAttemptID != nil && !session.ConfirmationAttemptID.IsZero() && !hasSaga {
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusNeedsAttention,
			nextAction:         ConfirmationNextActionShowPendingReview,
		}, nil
	}

	if hasSaga {
		switch sagaRow.Status {
		case model.ReconciliationRequiredSagaStatus, model.ManualReviewRequiredSagaStatus, model.FailedSagaStatus:
			return confirmationStatusProjection{
				confirmationStatus: ConfirmationStatusNeedsAttention,
				nextAction:         ConfirmationNextActionShowPendingReview,
			}, nil

		case model.RunningSagaStatus, model.WaitingSagaStatus:
			return waitProjection(ConfirmationStatusProcessing), nil
		}
	}

	switch status {
	case model.ConfirmingCheckoutSessionStatus:
		return waitProjection(ConfirmationStatusProcessing), nil

	case model.ProcessingCheckoutSessionStatus:
		return uc.projectProcessingConfirmation(ctx, session, now)

	case model.OpenCheckoutSessionStatus:
		return uc.projectOpenConfirmation(ctx, session, now)
	}

	return waitProjection(ConfirmationStatusProcessing), nil
}

func (uc *UseCase) projectOpenConfirmation(
	ctx context.Context,
	session model.CheckoutSession,
	now time.Time,
) (confirmationStatusProjection, error) {
	if session.ContractID == nil || session.ContractID.IsZero() {
		// Durable reconfirm signal after compensation / generation invalidation:
		// ReopenConfirmingToOpen clears confirmation_attempt_id and decision_id
		// while bumping evaluation_generation. First-time on_start stays at
		// generation 1 with no Decision and must not look like reconfirm.
		if session.EvaluationGeneration > 1 {
			return confirmationStatusProjection{
				confirmationStatus: ConfirmationStatusReconfirmRequired,
				nextAction:         ConfirmationNextActionReconfirm,
			}, nil
		}

		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusNotStarted,
			nextAction:         ConfirmationNextActionStartConfirm,
		}, nil
	}

	contract, attempt, paymentRequired, loadErr := uc.loadCheckoutPaymentContext(ctx, session)
	if loadErr != nil {
		return confirmationStatusProjection{}, loadErr
	}

	if !paymentRequired {
		// Merchant-funding (and similar) completion is authoritative only when
		// CheckoutSession itself is completed. An active Contract alone is not
		// a customer-facing success signal while the session is still open.
		return waitProjection(ConfirmationStatusProcessing), nil
	}

	if attempt != nil &&
		(attempt.State == model.FailedPaymentAttemptState ||
			attempt.State == model.CanceledPaymentAttemptState) &&
		contract.Status == model.PaymentPendingContractStatus {
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusRetryAvailable,
			nextAction:         ConfirmationNextActionRetryPayment,
		}, nil
	}

	if attempt != nil &&
		(attempt.State == model.ReconciliationRequiredPaymentAttemptState ||
			attempt.State == model.ManualReviewRequiredPaymentAttemptState) {
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusNeedsAttention,
			nextAction:         ConfirmationNextActionShowPendingReview,
		}, nil
	}

	if attempt != nil &&
		(attempt.State == model.CreatedPaymentAttemptState ||
			attempt.State == model.SubmittedPaymentAttemptState) {
		return projectPaymentContinuation(contract, *attempt, now), nil
	}

	return waitProjection(ConfirmationStatusProcessing), nil
}

func (uc *UseCase) projectProcessingConfirmation(
	ctx context.Context,
	session model.CheckoutSession,
	now time.Time,
) (confirmationStatusProjection, error) {
	if session.ContractID == nil || session.ContractID.IsZero() {
		return waitProjection(ConfirmationStatusProcessing), nil
	}

	contract, attempt, paymentRequired, loadErr := uc.loadCheckoutPaymentContext(ctx, session)
	if loadErr != nil {
		return confirmationStatusProjection{}, loadErr
	}

	if !paymentRequired {
		// Do not project completed from Contract activation alone. Checkout
		// completion remains the authoritative customer-facing success.
		return waitProjection(ConfirmationStatusProcessing), nil
	}

	if attempt == nil {
		return waitProjection(ConfirmationStatusProcessing), nil
	}

	switch attempt.State {
	case model.FailedPaymentAttemptState, model.CanceledPaymentAttemptState:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusRetryAvailable,
			nextAction:         ConfirmationNextActionRetryPayment,
		}, nil

	case model.ReconciliationRequiredPaymentAttemptState,
		model.ManualReviewRequiredPaymentAttemptState:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusNeedsAttention,
			nextAction:         ConfirmationNextActionShowPendingReview,
		}, nil

	case model.SucceededPaymentAttemptState:
		// PaymentAttempt success is not Checkout completion. Keep polling until
		// the authoritative CheckoutSession status becomes completed (or an
		// operator path resolves an inconsistent lag).
		return waitProjection(ConfirmationStatusPaymentProcessing), nil

	case model.CreatedPaymentAttemptState, model.SubmittedPaymentAttemptState:
		return projectPaymentContinuation(contract, *attempt, now), nil
	}

	return waitProjection(ConfirmationStatusProcessing), nil
}

func projectPaymentContinuation(
	contract model.Contract,
	attempt model.PaymentAttempt,
	now time.Time,
) confirmationStatusProjection {
	switch classifyPaymentContinuation(contract, attempt, now) {
	case paymentContinuationUsable:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusPaymentActionRequired,
			nextAction:         ConfirmationNextActionResumePayment,
			retryAfterSeconds:  intPtr(defaultConfirmationStatusPollSeconds),
		}

	case paymentContinuationMismatch:
		return confirmationStatusProjection{
			confirmationStatus: ConfirmationStatusNeedsAttention,
			nextAction:         ConfirmationNextActionShowPendingReview,
		}

	case paymentContinuationExpired, paymentContinuationIncomplete, paymentContinuationIneligible:
		// Incomplete/expired metadata is recoverable by Confirm/replay; do not
		// invent payment failure or claim a usable payment link.
		return waitProjection(ConfirmationStatusPaymentProcessing)

	default:
		return waitProjection(ConfirmationStatusPaymentProcessing)
	}
}

// classifyPaymentContinuation applies the same Contract/PaymentAttempt session
// completeness and identity agreement rules used by contract payment creation.
func classifyPaymentContinuation(
	contract model.Contract,
	attempt model.PaymentAttempt,
	now time.Time,
) paymentContinuationClass {
	if !contract.Status.AllowsCustomerPaymentContinuation() {
		return paymentContinuationIneligible
	}

	if attempt.State != model.CreatedPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState {
		return paymentContinuationIneligible
	}

	contractComplete := model.ContractHasFinancialCheckoutPaymentSession(contract)
	attemptComplete := model.PaymentAttemptHasFinancialCheckoutSession(attempt)

	switch {
	case !contractComplete || !attemptComplete:
		// One side missing fields: recoverable incomplete persistence, not a
		// definitive financial identity conflict.
		if paymentSessionIdentityConflict(contract, attempt) {
			return paymentContinuationMismatch
		}

		return paymentContinuationIncomplete

	case !model.CheckoutContractSessionMatchesAttempt(contract, attempt):
		return paymentContinuationMismatch

	case !contract.FinancialPaymentExpiresAt.UTC().After(now.UTC()):
		return paymentContinuationExpired

	default:
		return paymentContinuationUsable
	}
}

// paymentContinuationBlocksCustomerOTP reports whether a continuation class
// must refuse customer OTP issuance/consume. Incomplete is allowed so an
// authorized Saga recovery can still establish the first durable payment
// session. Expired means the only persisted gateway link is past its deadline
// — issuing OTP would only burn SMS and send budget without a usable URL.
// Local expiry is not treated as authoritative financial failure elsewhere.
func paymentContinuationBlocksCustomerOTP(class paymentContinuationClass) bool {
	switch class {
	case paymentContinuationUsable, paymentContinuationIncomplete:
		return false

	case paymentContinuationExpired,
		paymentContinuationMismatch,
		paymentContinuationIneligible:
		return true

	default:
		return true
	}
}

// paymentSessionIdentityConflict detects durable identifier disagreement when
// both sides expose the compared field. Missing fields alone are incomplete.
func paymentSessionIdentityConflict(contract model.Contract, attempt model.PaymentAttempt) bool {
	if nonemptyStringPtr(contract.FinancialPaymentIntentID) &&
		nonemptyStringPtr(attempt.FinancialPaymentIntentID) &&
		*contract.FinancialPaymentIntentID != *attempt.FinancialPaymentIntentID {
		return true
	}

	if nonemptyStringPtr(contract.FinancialPaymentSessionID) &&
		nonemptyStringPtr(attempt.FinancialPaymentSessionID) &&
		*contract.FinancialPaymentSessionID != *attempt.FinancialPaymentSessionID {
		return true
	}

	if nonemptyStringPtr(contract.FinancialPaymentLink) &&
		nonemptyStringPtr(attempt.PaymentURL) &&
		*contract.FinancialPaymentLink != *attempt.PaymentURL {
		return true
	}

	if contract.FinancialPaymentExpiresAt != nil &&
		attempt.FinancialPaymentExpiresAt != nil &&
		!contract.FinancialPaymentExpiresAt.Equal(*attempt.FinancialPaymentExpiresAt) {
		return true
	}

	if nonemptyStringPtr(contract.FinancialPaymentAttemptID) &&
		nonemptyStringPtr(attempt.FinancialPaymentAttemptID) &&
		*contract.FinancialPaymentAttemptID != *attempt.FinancialPaymentAttemptID {
		return true
	}

	return false
}

func nonemptyStringPtr(value *string) bool {
	return value != nil && *value != ""
}

func (uc *UseCase) loadCheckoutPaymentContext(
	ctx context.Context,
	session model.CheckoutSession,
) (model.Contract, *model.PaymentAttempt, bool, error) {
	var (
		contract        model.Contract
		attempt         *model.PaymentAttempt
		paymentRequired bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := uc.contract.Conn(conn)
		row, err := contractQ.GetByID(ctx, *session.ContractID)
		if err != nil {
			return err
		}

		contract = row
		return nil
	})
	if err != nil {
		return model.Contract{}, nil, false, err
	}

	required, modeErr := uc.contractRequiresInitialPayment(ctx, contract)
	if modeErr != nil {
		return model.Contract{}, nil, false, modeErr
	}

	paymentRequired = required
	if !paymentRequired {
		return contract, nil, false, nil
	}

	loaded, loadErr := uc.loadCheckoutPaymentAttempt(ctx, contract.ID)
	if loadErr != nil {
		if cerr.IsCode(loadErr, cerr.CodeResourceNotFound) {
			return contract, nil, true, nil
		}

		return model.Contract{}, nil, false, loadErr
	}

	attempt = &loaded
	return contract, attempt, true, nil
}

func (uc *UseCase) lookupConfirmationSaga(
	ctx context.Context,
	session model.CheckoutSession,
) (model.SagaInstance, bool, error) {
	if session.ConfirmationAttemptID == nil || session.ConfirmationAttemptID.IsZero() {
		return model.SagaInstance{}, false, nil
	}

	instance, err := uc.findConfirmationSagaBySession(ctx, session)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.SagaInstance{}, false, nil
		}

		return model.SagaInstance{}, false, err
	}

	return instance, true, nil
}

func waitProjection(status string) confirmationStatusProjection {
	return confirmationStatusProjection{
		confirmationStatus: status,
		nextAction:         ConfirmationNextActionWait,
		retryAfterSeconds:  intPtr(defaultConfirmationStatusPollSeconds),
	}
}

func intPtr(value int) *int {
	return &value
}
