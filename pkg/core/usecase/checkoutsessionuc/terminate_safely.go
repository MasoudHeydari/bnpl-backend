package checkoutsessionuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// TerminateCheckoutSafelyInput asks BNPL to release local checkout gates only
// when absence of successful or unknown financial effect is proven from durable
// local evidence (and any prior out-of-transaction FC lookups already applied).
type TerminateCheckoutSafelyInput struct {
	SessionID model.CheckoutSessionID
	// Reason is required when TerminalStatus is canceled.
	Reason model.CheckoutCancellationReason
	// TerminalStatus must be canceled or expired.
	TerminalStatus model.CheckoutSessionStatus
}

// TerminateCheckoutSafelyOutput reports the session after a safe termination
// attempt. AlreadyTerminal is true when the session was already canceled or
// expired (idempotent success).
type TerminateCheckoutSafelyOutput struct {
	Session         model.CheckoutSession
	AlreadyTerminal bool
}

// TerminateCheckoutSafely releases payment_pending and active merchant-order
// checkout limits when local evidence proves there is no successful or unknown
// Financial Core effect for the purchase.
//
// It never calls Financial Core. Callers that hold a payment_intent_id with an
// unresolved attempt must look up status first and apply an authoritative
// result before invoking this operation.
//
// Concurrency: session → contract → payment attempt → saga lock order. The
// session CAS clears confirmation_attempt_id so a prior Saga cannot revive
// processing after commit.
func (uc *UseCase) TerminateCheckoutSafely(
	ctx context.Context,
	input TerminateCheckoutSafelyInput,
) (TerminateCheckoutSafelyOutput, error) {
	const op = "checkoutsessionuc.UseCase.TerminateCheckoutSafely"

	if input.SessionID.IsZero() {
		return TerminateCheckoutSafelyOutput{}, fmt.Errorf("%s: session id is required", op)
	}

	switch input.TerminalStatus {
	case model.CanceledCheckoutSessionStatus:
		if !input.Reason.IsValid() {
			return TerminateCheckoutSafelyOutput{}, cerr.InvalidArgument(
				fmt.Errorf("%s: cancellation reason is required", op),
			)
		}

	case model.ExpiredCheckoutSessionStatus:
		// Expiry carries no cancellation reason.

	default:
		return TerminateCheckoutSafelyOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: terminal status must be canceled or expired", op),
		)
	}

	var output TerminateCheckoutSafelyOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			now := uc.clock.Now().UTC()
			result, termErr := uc.terminateCheckoutSafelyTx(ctx, tx, input, now)
			if termErr != nil {
				return termErr
			}

			output = result
			return nil
		})
	})
	if err != nil {
		return TerminateCheckoutSafelyOutput{}, err
	}

	return output, nil
}

func (uc *UseCase) terminateCheckoutSafelyTx(
	ctx context.Context,
	tx repo.Tx,
	input TerminateCheckoutSafelyInput,
	now time.Time,
) (TerminateCheckoutSafelyOutput, error) {
	const op = "checkoutsessionuc.UseCase.terminateCheckoutSafelyTx"

	sessionQ := uc.checkoutSession.Tx(tx)
	locked, err := sessionQ.LockByID(ctx, input.SessionID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return TerminateCheckoutSafelyOutput{}, checkoutSessionNotFoundError(err)
		}

		return TerminateCheckoutSafelyOutput{}, err
	}

	switch locked.Status {
	case model.CanceledCheckoutSessionStatus, model.ExpiredCheckoutSessionStatus:
		return TerminateCheckoutSafelyOutput{Session: locked, AlreadyTerminal: true}, nil

	case model.CompletedCheckoutSessionStatus, model.UnavailableCheckoutSessionStatus:
		return TerminateCheckoutSafelyOutput{}, checkoutSessionNotCancelableError(
			fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
		)

	case model.OpenCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.ProcessingCheckoutSessionStatus:
		// continue

	default:
		return TerminateCheckoutSafelyOutput{}, checkoutSessionNotCancelableError(
			fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
		)
	}

	evidence, err := uc.evaluateSafeTerminationEvidence(ctx, tx, locked)
	if err != nil {
		return TerminateCheckoutSafelyOutput{}, err
	}

	if !evidence.Safe {
		uc.observer.CheckoutSafeTerminationRejected(ctx, locked.ID, evidence.BlockReason)

		return TerminateCheckoutSafelyOutput{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: %s", op, evidence.BlockReason),
		)
	}

	if locked.ContractID != nil {
		if err := uc.terminateLocalPaymentPendingContract(ctx, tx, *locked.ContractID, now); err != nil {
			return TerminateCheckoutSafelyOutput{}, err
		}
	}

	if err := uc.fenceConfirmationSaga(ctx, tx, locked, now); err != nil {
		return TerminateCheckoutSafelyOutput{}, err
	}

	var terminated model.CheckoutSession
	switch input.TerminalStatus {
	case model.CanceledCheckoutSessionStatus:
		if transitionErr := locked.Status.ValidateTransitionTo(model.CanceledCheckoutSessionStatus); transitionErr != nil {
			return TerminateCheckoutSafelyOutput{}, cerr.InternalError(
				fmt.Errorf("%s: %w", op, transitionErr),
			)
		}

		terminated, err = sessionQ.MarkCanceledSafely(ctx, locked.ID, input.Reason, now)
		if err != nil {
			return TerminateCheckoutSafelyOutput{}, err
		}

		if enqueueErr := uc.lifecycle.Enqueue(
			ctx,
			tx,
			terminated,
			checkoutlifecycle.EventTypeCanceled,
			now,
		); enqueueErr != nil {
			return TerminateCheckoutSafelyOutput{}, enqueueErr
		}

	case model.ExpiredCheckoutSessionStatus:
		if transitionErr := locked.Status.ValidateTransitionTo(model.ExpiredCheckoutSessionStatus); transitionErr != nil {
			return TerminateCheckoutSafelyOutput{}, cerr.InternalError(
				fmt.Errorf("%s: %w", op, transitionErr),
			)
		}

		terminated, err = sessionQ.MarkExpiredSafely(ctx, locked.ID, now)
		if err != nil {
			return TerminateCheckoutSafelyOutput{}, err
		}

		if enqueueErr := uc.lifecycle.Enqueue(
			ctx,
			tx,
			terminated,
			checkoutlifecycle.EventTypeExpired,
			now,
		); enqueueErr != nil {
			return TerminateCheckoutSafelyOutput{}, enqueueErr
		}

	default:
		return TerminateCheckoutSafelyOutput{}, cerr.InternalError(
			fmt.Errorf("%s: unsupported terminal status %q", op, input.TerminalStatus),
		)
	}

	uc.observer.CheckoutTerminatedSafely(ctx, terminated.ID, string(input.TerminalStatus), string(input.Reason))

	return TerminateCheckoutSafelyOutput{Session: terminated}, nil
}

type safeTerminationEvidence struct {
	Safe        bool
	BlockReason string
}

func (uc *UseCase) evaluateSafeTerminationEvidence(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
) (safeTerminationEvidence, error) {
	const op = "checkoutsessionuc.UseCase.evaluateSafeTerminationEvidence"

	if session.ContractID == nil {
		// Confirming may have prepared a Contract by Decision before Link.
		// Absence of session.ContractID alone is not proof no local contract
		// (and therefore no FC call) exists for this purchase.
		if session.DecisionID != nil && !session.DecisionID.IsZero() {
			contractQ := uc.contract.Tx(tx)
			_, findErr := contractQ.GetByDecisionID(ctx, *session.DecisionID)
			if findErr == nil {
				return safeTerminationEvidence{
					Safe:        false,
					BlockReason: "decision already has a prepared contract not yet linked to the session",
				}, nil
			}

			if !cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
				return safeTerminationEvidence{}, findErr
			}
		}

		return safeTerminationEvidence{Safe: true}, nil
	}

	contractQ := uc.contract.Tx(tx)
	contractRow, err := contractQ.LockByID(ctx, *session.ContractID)
	if err != nil {
		return safeTerminationEvidence{}, err
	}

	switch contractRow.Status {
	case model.CanceledContractStatus, model.ExpiredContractStatus:
		return safeTerminationEvidence{Safe: true}, nil

	case model.PaymentPendingContractStatus:
		// continue

	default:
		return safeTerminationEvidence{
			Safe:        false,
			BlockReason: fmt.Sprintf("contract status %q blocks safe termination", contractRow.Status),
		}, nil
	}

	if hasConfirmedFinancialIdentity(contractRow) {
		return safeTerminationEvidence{
			Safe:        false,
			BlockReason: "contract carries confirmed Financial Core identity",
		}, nil
	}

	paymentAttemptQ := uc.paymentAttempt.Tx(tx)
	attempt, err := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx,
		contractRow.ID,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if err != nil {
		if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return safeTerminationEvidence{}, err
		}

		requiresPayment, modeErr := uc.contractRequiresInitialPaymentInTx(ctx, tx, contractRow)
		if modeErr != nil {
			return safeTerminationEvidence{}, fmt.Errorf("%s: resolve activation mode: %w", op, modeErr)
		}

		if !requiresPayment {
			// Merchant-funding path with no payment attempt: absence of an
			// attempt does not prove RecordMerchantEntitlement never ran.
			return safeTerminationEvidence{
				Safe:        false,
				BlockReason: "merchant funding may have an unknown Financial Core entitlement effect",
			}, nil
		}

		// Initial-payment contract with no attempt is a true local orphan.
		return safeTerminationEvidence{Safe: true}, nil
	}

	attempt, err = paymentAttemptQ.LockByIDWithItems(ctx, attempt.ID)
	if err != nil {
		return safeTerminationEvidence{}, err
	}

	switch attempt.State {
	case model.FailedPaymentAttemptState, model.CanceledPaymentAttemptState:
		if !attempt.FinancialOutcomeResolved {
			return safeTerminationEvidence{
				Safe:        false,
				BlockReason: "payment attempt terminal without resolved financial outcome",
			}, nil
		}

		return safeTerminationEvidence{Safe: true}, nil

	case model.CreatedPaymentAttemptState:
		// A create reservation (attempt_count > 0) or active lease means the
		// CreateCheckoutPayment RPC may have been sent. Treat as unknown.
		if attempt.FinancialSessionCreateAttemptCount > 0 {
			return safeTerminationEvidence{
				Safe:        false,
				BlockReason: "payment session create may have been dispatched",
			}, nil
		}

		if attempt.FinancialSessionCreateLeaseUntil != nil &&
			!attempt.FinancialSessionCreateLeaseUntil.IsZero() {
			return safeTerminationEvidence{
				Safe:        false,
				BlockReason: "payment session create lease was issued",
			}, nil
		}

		return safeTerminationEvidence{Safe: true}, nil

	case model.SubmittedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
		model.ManualReviewRequiredPaymentAttemptState,
		model.SucceededPaymentAttemptState,
		model.ExpiredPaymentAttemptState:
		return safeTerminationEvidence{
			Safe:        false,
			BlockReason: fmt.Sprintf("payment attempt state %q has unresolved or successful financial effect", attempt.State),
		}, nil

	default:
		return safeTerminationEvidence{}, cerr.InternalError(
			fmt.Errorf("%s: unknown checkout payment attempt state %q", op, attempt.State),
		)
	}
}

func hasConfirmedFinancialIdentity(contractRow model.Contract) bool {
	if contractRow.FinancialMerchantEntitlementID != nil &&
		strings.TrimSpace(*contractRow.FinancialMerchantEntitlementID) != "" {
		return true
	}

	if contractRow.FinancialReceivableID != nil &&
		strings.TrimSpace(*contractRow.FinancialReceivableID) != "" {
		return true
	}

	if contractRow.FinancialInvoiceID != nil &&
		strings.TrimSpace(*contractRow.FinancialInvoiceID) != "" {
		return true
	}

	if contractRow.FinancialPaymentIntentID != nil &&
		strings.TrimSpace(*contractRow.FinancialPaymentIntentID) != "" {
		return true
	}

	if contractRow.SuccessfulFinancialPaymentAttemptID != nil &&
		strings.TrimSpace(*contractRow.SuccessfulFinancialPaymentAttemptID) != "" {
		return true
	}

	return false
}

func (uc *UseCase) contractRequiresInitialPaymentInTx(
	ctx context.Context,
	tx repo.Tx,
	contractRow model.Contract,
) (bool, error) {
	const op = "checkoutsessionuc.UseCase.contractRequiresInitialPaymentInTx"

	decisionQ := uc.decisionRepo.Tx(tx)
	decision, err := decisionQ.GetByID(ctx, contractRow.DecisionID)
	if err != nil {
		return false, err
	}

	selected, err := uc.checkoutPolicies.Resolve(policy.Ref{
		ID:      decision.PolicyID,
		Version: decision.PolicyVersion,
	})
	if err != nil {
		return false, fmt.Errorf("%s: resolve checkout policy: %w", op, err)
	}

	return selected.Terms().ActivationMode == checkout.ActivationAfterInitialPayment, nil
}

func (uc *UseCase) terminateLocalPaymentPendingContract(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	const op = "checkoutsessionuc.UseCase.terminateLocalPaymentPendingContract"

	contractQ := uc.contract.Tx(tx)
	paymentAttemptQ := uc.paymentAttempt.Tx(tx)
	installmentQ := uc.installment.Tx(tx)
	installmentChargeQ := uc.installmentCharge.Tx(tx)

	locked, err := contractQ.LockByID(ctx, contractID)
	if err != nil {
		return err
	}

	switch locked.Status {
	case model.CanceledContractStatus, model.ExpiredContractStatus:
		return nil

	case model.PaymentPendingContractStatus:
		// continue

	default:
		return checkoutSessionNotCancelableError(
			fmt.Errorf("%s: contract status is %q", op, locked.Status),
		)
	}

	if _, err = contractQ.MarkCanceledFromPaymentPending(ctx, locked.ID, now, now); err != nil {
		return err
	}

	if err = cancelUnsubmittedCheckoutPayment(ctx, paymentAttemptQ, locked.ID, now); err != nil {
		return err
	}

	if _, err = installmentQ.ExpireOfferedByContractID(ctx, locked.ID, now); err != nil {
		return err
	}

	if _, err = installmentChargeQ.CancelOfferedByContractID(ctx, locked.ID, now); err != nil {
		return err
	}

	return nil
}

func (uc *UseCase) fenceConfirmationSaga(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
	now time.Time,
) error {
	const op = "checkoutsessionuc.UseCase.fenceConfirmationSaga"

	if session.ConfirmationAttemptID == nil || session.ConfirmationAttemptID.IsZero() {
		return nil
	}

	businessKey := model.SagaBusinessKey(session.ConfirmationAttemptID.String())
	sagaQ := uc.confirmationSagaRepo.Tx(tx)

	instance, err := sagaQ.GetByTypeAndBusinessKey(
		ctx,
		checkoutConfirmationSagaType,
		businessKey,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		return err
	}

	if instance.Status.IsTerminal() {
		return nil
	}

	locked, err := sagaQ.LockByID(ctx, instance.ID)
	if err != nil {
		return err
	}

	if locked.Status.IsTerminal() {
		return nil
	}

	if err := locked.Status.ValidateTransitionTo(model.FailedSagaStatus); err != nil {
		return cerr.Conflict(
			fmt.Errorf("%s: cannot fence saga %s from status %q: %w", op, locked.ID, locked.Status, err),
		)
	}

	reason := "checkout_safe_termination"
	finishedAt := now
	_, err = sagaQ.Update(
		ctx,
		locked.ID,
		locked.Version,
		locked.Phase,
		model.FailedSagaStatus,
		locked.CurrentStep,
		locked.StepAttemptCount,
		nil,
		&reason,
		&finishedAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("%s: fence confirmation saga: %w", op, err)
	}

	return nil
}
