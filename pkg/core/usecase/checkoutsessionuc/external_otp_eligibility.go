package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// otpContinuationDeps are the Saga / Contract / PaymentAttempt rows locked for
// hosted-checkout OTP continuation. CheckoutSession is deliberately omitted —
// callers lock CS after these deps so the canonical OTP×Saga×FC order is:
//
//	Saga → Contract → PaymentAttempt → CheckoutSession → OTPChallenge → RL(phone) → RL(merchant)
//
// Locking CS first then waiting on Saga (or Contract/PA) deadlocks with a
// concurrent confirmation step that already holds Saga and needs CS.
type otpContinuationDeps struct {
	contract model.Contract
	attempt  model.PaymentAttempt
	saga     *model.SagaInstance
}

// lockOTPContinuationDepsInTx locks continuation dependencies without locking
// CheckoutSession. When ConfirmationAttemptID is set it locks the confirmation
// Saga first, then Contract, then the checkout first-installment PaymentAttempt.
//
// Eligibility predicates (contract continuation, saga not terminal/review,
// PA not terminal/review/succeeded, session metadata match) are enforced under
// those locks. No network I/O occurs here.
func (uc *UseCase) lockOTPContinuationDepsInTx(
	ctx context.Context,
	tx repo.Tx,
	peeked model.CheckoutSession,
) (otpContinuationDeps, error) {
	const op = "checkoutsessionuc.UseCase.lockOTPContinuationDepsInTx"

	if peeked.ContractID == nil || peeked.ContractID.IsZero() {
		return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: linked contract required for status %q", op, peeked.Status),
		)
	}

	if peeked.CustomerID == nil || peeked.CustomerID.IsZero() {
		return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: customer binding is required", op),
		)
	}

	var deps otpContinuationDeps

	if peeked.ConfirmationAttemptID != nil && !peeked.ConfirmationAttemptID.IsZero() {
		businessKey, keyErr := confirmationSagaBusinessKey(peeked)
		if keyErr != nil {
			return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
				fmt.Errorf("%s: confirmation saga business key: %w", op, keyErr),
			)
		}

		sagaQ := uc.confirmationSagaRepo.Tx(tx)
		sagaRow, sagaErr := sagaQ.GetByTypeAndBusinessKey(
			ctx,
			checkoutConfirmationSagaType,
			businessKey,
		)
		if sagaErr != nil {
			// Confirm creates ConfirmationAttemptID and the Saga atomically.
			// A missing Saga under a nonzero attempt id is corruption — fail closed.
			if cerr.IsCode(sagaErr, cerr.CodeResourceNotFound) {
				return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
					fmt.Errorf("%s: confirmation saga missing for attempt %s", op, peeked.ConfirmationAttemptID),
				)
			}

			return otpContinuationDeps{}, sagaErr
		}

		lockedSaga, lockErr := sagaQ.LockByID(ctx, sagaRow.ID)
		if lockErr != nil {
			return otpContinuationDeps{}, lockErr
		}

		switch lockedSaga.Status {
		case model.ReconciliationRequiredSagaStatus,
			model.ManualReviewRequiredSagaStatus,
			model.FailedSagaStatus:
			return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
				fmt.Errorf("%s: saga status %q blocks otp issuance", op, lockedSaga.Status),
			)
		}

		deps.saga = &lockedSaga
	}

	contractQ := uc.contract.Tx(tx)
	contract, err := contractQ.LockByID(ctx, *peeked.ContractID)
	if err != nil {
		return otpContinuationDeps{}, err
	}

	if !contract.Status.AllowsCustomerPaymentContinuation() {
		return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: contract status %q is not eligible", op, contract.Status),
		)
	}

	deps.contract = contract

	attemptQ := uc.paymentAttempt.Tx(tx)
	attempt, attemptErr := attemptQ.GetByContractIDAndPurposeWithItems(
		ctx,
		contract.ID,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if attemptErr != nil {
		if cerr.IsCode(attemptErr, cerr.CodeResourceNotFound) {
			return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
				fmt.Errorf("%s: no checkout payment attempt is available", op),
			)
		}

		return otpContinuationDeps{}, attemptErr
	}

	lockedAttempt, lockErr := attemptQ.LockByIDWithItems(ctx, attempt.ID)
	if lockErr != nil {
		return otpContinuationDeps{}, lockErr
	}

	switch lockedAttempt.State {
	case model.ReconciliationRequiredPaymentAttemptState,
		model.ManualReviewRequiredPaymentAttemptState:
		return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: payment attempt requires operator attention", op),
		)

	case model.SucceededPaymentAttemptState:
		return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: payment attempt already succeeded", op),
		)
	}

	now := uc.clock.Now().UTC()

	// Presence of a checkout first-installment attempt is the local durable
	// signal that customer payment continuation applies. Do not open a nested
	// Conn to re-resolve policy while holding row locks.
	if lockedAttempt.State == model.CreatedPaymentAttemptState ||
		lockedAttempt.State == model.SubmittedPaymentAttemptState {
		class := classifyPaymentContinuation(contract, lockedAttempt, now)
		if paymentContinuationBlocksCustomerOTP(class) {
			return otpContinuationDeps{}, checkoutOTPIssuanceNotAllowedError(
				fmt.Errorf("%s: payment continuation class %v blocks otp", op, class),
			)
		}
	}

	deps.attempt = lockedAttempt

	return deps, nil
}

// assertInitialOTPIssuanceEligible enforces open-session OTP eligibility under
// the CheckoutSession row lock. No Saga/Contract/PA locks are required.
func (uc *UseCase) assertInitialOTPIssuanceEligible(
	locked model.CheckoutSession,
	now time.Time,
	op string,
) error {
	if locked.ContractID != nil && !locked.ContractID.IsZero() {
		return checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: initial otp cannot target a linked contract", op),
		)
	}

	if locked.CustomerID == nil || locked.CustomerID.IsZero() {
		return checkoutOfferNotReadyError(
			fmt.Errorf("%s: customer identity is not resolved", op),
		)
	}

	if locked.DecisionID == nil || locked.DecisionID.IsZero() {
		return checkoutOfferNotReadyError(
			fmt.Errorf("%s: financing decision is not prepared", op),
		)
	}

	if locked.IsExpiredAt(now) {
		return checkoutSessionExpiredError()
	}

	return nil
}

// assertContinuationSessionMatchesPeeked rejects a TOCTOU drift between the
// unlocked peek and the locked CheckoutSession used for OTP continuation.
func assertContinuationSessionMatchesPeeked(
	locked model.CheckoutSession,
	peeked model.CheckoutSession,
	op string,
) error {
	if locked.Status != peeked.Status {
		return checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: checkout session status changed from %q to %q", op, peeked.Status, locked.Status),
		)
	}

	if locked.EvaluationGeneration != peeked.EvaluationGeneration {
		return checkoutOfferReconfirmRequiredError(
			fmt.Errorf(
				"%s: evaluation generation changed from %d to %d",
				op,
				peeked.EvaluationGeneration,
				locked.EvaluationGeneration,
			),
		)
	}

	if !sameOptionalContractID(locked.ContractID, peeked.ContractID) {
		return checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: linked contract changed under peek", op),
		)
	}

	if !sameOptionalConfirmationAttemptID(locked.ConfirmationAttemptID, peeked.ConfirmationAttemptID) {
		return checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: confirmation attempt changed under peek", op),
		)
	}

	return nil
}

func sameOptionalContractID(a, b *model.ContractID) bool {
	switch {
	case a == nil && b == nil:
		return true

	case a == nil || b == nil:
		return false

	default:
		return *a == *b
	}
}

func sameOptionalConfirmationAttemptID(a, b *model.ConfirmationAttemptID) bool {
	switch {
	case a == nil && b == nil:
		return true

	case a == nil || b == nil:
		return false

	default:
		return *a == *b
	}
}

// evaluateOTPIssuanceBlockersOutsideTx is a read-only projection helper for
// otp_availability. It must not mutate state and must not call external
// services. OTP lock/send budgets are projected by the otp package; this helper
// covers checkout-business blockers only. Request-time rate limits remain
// authoritative on POST /otp.
func (uc *UseCase) evaluateOTPIssuanceBlockersOutsideTx(
	ctx context.Context,
	session model.CheckoutSession,
	status model.CheckoutSessionStatus,
	now time.Time,
) (blocked bool, err error) {
	switch status {
	case model.CompletedCheckoutSessionStatus,
		model.CanceledCheckoutSessionStatus,
		model.ExpiredCheckoutSessionStatus,
		model.UnavailableCheckoutSessionStatus:
		return true, nil
	}

	switch status {
	case model.OpenCheckoutSessionStatus:
		if session.ContractID != nil && !session.ContractID.IsZero() {
			return true, nil
		}

		if session.CustomerID == nil || session.DecisionID == nil || session.DecisionID.IsZero() {
			return true, nil
		}

		return false, nil

	case model.ConfirmingCheckoutSessionStatus, model.ProcessingCheckoutSessionStatus:
		if session.ContractID == nil || session.ContractID.IsZero() {
			return true, nil
		}

		if session.ConfirmationAttemptID != nil && !session.ConfirmationAttemptID.IsZero() {
			sagaRow, hasSaga, sagaErr := uc.lookupConfirmationSaga(ctx, session)
			if sagaErr != nil {
				return false, sagaErr
			}

			// Missing Saga under a nonzero ConfirmationAttemptID is an invariant
			// break: block customer OTP rather than pretending continuation works.
			if !hasSaga {
				return true, nil
			}

			switch sagaRow.Status {
			case model.ReconciliationRequiredSagaStatus,
				model.ManualReviewRequiredSagaStatus,
				model.FailedSagaStatus:
				return true, nil
			}
		}

		contract, attempt, paymentRequired, loadErr := uc.loadCheckoutPaymentContext(ctx, session)
		if loadErr != nil {
			return false, loadErr
		}

		if !paymentRequired || attempt == nil {
			return true, nil
		}

		if !contract.Status.AllowsCustomerPaymentContinuation() {
			return true, nil
		}

		switch attempt.State {
		case model.ReconciliationRequiredPaymentAttemptState,
			model.ManualReviewRequiredPaymentAttemptState,
			model.SucceededPaymentAttemptState:
			return true, nil
		}

		if attempt.State == model.CreatedPaymentAttemptState ||
			attempt.State == model.SubmittedPaymentAttemptState {
			if paymentContinuationBlocksCustomerOTP(
				classifyPaymentContinuation(contract, *attempt, now),
			) {
				return true, nil
			}
		}

		return false, nil

	default:
		return true, nil
	}
}
