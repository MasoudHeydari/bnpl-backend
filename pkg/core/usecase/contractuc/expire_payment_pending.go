package contractuc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ExpireDuePaymentPendingContractsOutput reports payment-pending contract expiry results.
type ExpireDuePaymentPendingContractsOutput struct {
	// ExpiredContractCount is the number of payment_pending contracts expired.
	ExpiredContractCount int64
	// ExpiredInstallmentCount is the number of offered installments expired.
	ExpiredInstallmentCount int64
	// CanceledChargeCount is the number of installment charges canceled.
	CanceledChargeCount int64
	// FinalizedPaymentAttemptCount is the number of payment attempts finalized.
	FinalizedPaymentAttemptCount int64
}

// ExpireDuePaymentPendingContracts expires payment_pending contracts whose operational
// expiry eligibility time has passed.
func (uc *UseCase) ExpireDuePaymentPendingContracts(
	ctx context.Context,
) (ExpireDuePaymentPendingContractsOutput, error) {
	const op = "contractuc.UseCase.ExpireDuePaymentPendingContracts"

	now := uc.clock.Now().UTC()
	output := ExpireDuePaymentPendingContractsOutput{}

	var contractIDs []model.ContractID

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					contractQ := uc.contract.Tx(tx)
					ids, listErr := contractQ.ListDuePaymentPendingContractIDs(
						ctx,
						now,
						uc.PaymentResultProcessingGracePeriod,
						uc.ExpiryBatchSize,
					)
					if listErr != nil {
						return listErr
					}

					contractIDs = ids

					return nil
				},
			)
		},
	)
	if err != nil {
		return ExpireDuePaymentPendingContractsOutput{}, err
	}

	var batchErr error
	for _, contractID := range contractIDs {
		if err := ctx.Err(); err != nil {
			batchErr = errors.Join(batchErr, err)
			break
		}

		counts, expired, expireErr := uc.expirePaymentPendingContractByID(ctx, contractID, now)
		if expireErr != nil {
			batchErr = errors.Join(
				batchErr,
				fmt.Errorf("%s: expire payment pending contract %s: %w", op, contractID, expireErr),
			)
			continue
		}

		if !expired {
			continue
		}

		output.ExpiredContractCount++
		output.ExpiredInstallmentCount += counts.expiredInstallments
		output.CanceledChargeCount += counts.canceledCharges
		output.FinalizedPaymentAttemptCount += counts.finalizedPaymentAttempts
	}

	return output, batchErr
}

type expirePaymentPendingCounts struct {
	expiredInstallments      int64
	canceledCharges          int64
	finalizedPaymentAttempts int64
}

func (uc *UseCase) expirePaymentPendingContractByID(
	ctx context.Context,
	contractID model.ContractID,
	now time.Time,
) (expirePaymentPendingCounts, bool, error) {
	var (
		counts  expirePaymentPendingCounts
		expired bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					contractQ := uc.contract.Tx(tx)
					installmentQ := uc.installment.Tx(tx)
					installmentChargeQ := uc.installmentCharge.Tx(tx)
					paymentAttemptQ := uc.paymentAttempt.Tx(tx)
					decisionQ := uc.decision.Tx(tx)
					auditOutboxQ := uc.outbox.Tx(tx)

					contractRow, lockErr := contractQ.LockByID(ctx, contractID)
					if lockErr != nil {
						return lockErr
					}

					var expireErr error
					counts, expired, expireErr = uc.expirePaymentPendingContract(
						ctx,
						contractQ,
						installmentQ,
						installmentChargeQ,
						paymentAttemptQ,
						decisionQ,
						auditOutboxQ,
						contractRow,
						now,
					)

					return expireErr
				},
			)
		},
	)
	if err != nil {
		return expirePaymentPendingCounts{}, false, err
	}

	return counts, expired, nil
}

func (uc *UseCase) expirePaymentPendingContract(
	ctx context.Context,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	decisionQ repo.DecisionTxQueryer,
	auditOutboxQ repo.OutboxTxQueryer,
	contractRow model.Contract,
	now time.Time,
) (expirePaymentPendingCounts, bool, error) {
	if contractRow.Status == model.ActiveContractStatus ||
		contractRow.Status == model.ExpiredContractStatus {
		return expirePaymentPendingCounts{}, false, nil
	}

	if contractRow.Status != model.PaymentPendingContractStatus {
		return expirePaymentPendingCounts{}, false, nil
	}

	if !contractRow.IsExpiryEligible(now, uc.PaymentResultProcessingGracePeriod) {
		return expirePaymentPendingCounts{}, false, nil
	}

	// payment_pending is the durable pre-activation gate for both initial-payment
	// and merchant-funding policies. Never release that gate merely because local
	// wall-clock expiry elapsed while an externally submitted payment or FC
	// merchant-funding recognition has an unknown outcome.
	safeToExpire, err := uc.paymentPendingContractSafeToExpire(ctx, paymentAttemptQ, decisionQ, contractRow)
	if err != nil {
		return expirePaymentPendingCounts{}, false, err
	}

	if !safeToExpire {
		return expirePaymentPendingCounts{}, false, nil
	}

	if err := contractRow.Status.ValidateTransitionTo(model.ExpiredContractStatus); err != nil {
		return expirePaymentPendingCounts{}, false, cerr.InternalError(err)
	}

	if _, err := contractQ.MarkExpired(ctx, contractRow.ID, now); err != nil {
		if cerr.IsCode(err, cerr.CodeConflict) {
			return expirePaymentPendingCounts{}, false, nil
		}

		return expirePaymentPendingCounts{}, false, err
	}

	paymentAttemptTransition, err := uc.finalizeCheckoutPaymentAttemptForExpiredContract(
		ctx,
		paymentAttemptQ,
		contractRow.ID,
		now,
	)
	if err != nil {
		return expirePaymentPendingCounts{}, false, err
	}

	expiredInstallments, err := installmentQ.ExpireOfferedByContractID(ctx, contractRow.ID, now)
	if err != nil {
		return expirePaymentPendingCounts{}, false, err
	}

	canceledCharges, err := installmentChargeQ.CancelOfferedByContractID(ctx, contractRow.ID, now)
	if err != nil {
		return expirePaymentPendingCounts{}, false, err
	}

	counts := expirePaymentPendingCounts{
		expiredInstallments:      expiredInstallments,
		canceledCharges:          canceledCharges,
		finalizedPaymentAttempts: paymentAttemptTransition.affectedCount,
	}

	facts := expirePaymentPendingAuditFacts{
		Contract:                       contractRow,
		ExpiredInstallmentCount:        expiredInstallments,
		CanceledChargeCount:            canceledCharges,
		PaymentAttemptID:               paymentAttemptTransition.paymentAttemptID,
		PreviousPaymentAttemptState:    paymentAttemptTransition.previousState,
		CurrentPaymentAttemptState:     paymentAttemptTransition.currentState,
		PaymentAttemptTransitionReason: paymentAttemptTransition.reason,
	}
	if err := uc.insertExpirePaymentPendingAuditOutbox(
		ctx,
		auditOutboxQ,
		facts,
		now,
	); err != nil {
		return expirePaymentPendingCounts{}, false, err
	}

	return counts, true, nil
}

// paymentPendingContractSafeToExpire reports whether local expiry may release
// the contract's pre-activation gate without risking a second concurrent
// Financial Core fulfillment.
//
// payment_pending does not imply a PaymentAttempt exists. Merchant-funding
// contracts wait in payment_pending with no attempt while FC recognition is
// retried; those must not be expired as orphans. Initial-payment contracts
// without an attempt are true orphans and may expire. Submitted /
// reconciliation / manual-review / succeeded attempt states are never safe:
// their Financial Core outcome is or was externally observable.
func (uc *UseCase) paymentPendingContractSafeToExpire(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	decisionQ repo.DecisionTxQueryer,
	contractRow model.Contract,
) (bool, error) {
	const op = "contractuc.UseCase.paymentPendingContractSafeToExpire"

	attempt, err := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx, contractRow.ID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			requiresPayment, modeErr := uc.contractRequiresInitialPaymentInTx(ctx, decisionQ, contractRow)
			if modeErr != nil {
				return false, fmt.Errorf("%s: resolve activation mode: %w", op, modeErr)
			}

			// No attempt + merchant-funding policy ⇒ waiting for / retrying FC
			// merchant funding. Do not expire.
			return requiresPayment, nil
		}

		return false, err
	}

	// Lock the attempt after resolving it by contract/purpose. The contract row is
	// already locked by the caller, which matches the lock order used by payment
	// result processing (contract -> payment attempt) and prevents the attempt from
	// becoming submitted between this safety check and local expiry.
	attempt, err = paymentAttemptQ.LockByIDWithItems(ctx, attempt.ID)
	if err != nil {
		return false, err
	}

	switch attempt.State {
	case model.CreatedPaymentAttemptState,
		model.FailedPaymentAttemptState,
		model.CanceledPaymentAttemptState:
		return true, nil

	case model.SubmittedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
		model.ManualReviewRequiredPaymentAttemptState,
		model.SucceededPaymentAttemptState:
		return false, nil

	default:
		return false, fmt.Errorf("unknown checkout payment attempt state %q", attempt.State)
	}
}

type paymentAttemptExpiryTransition struct {
	affectedCount    int64
	paymentAttemptID *model.PaymentAttemptID
	previousState    *model.PaymentAttemptState
	currentState     *model.PaymentAttemptState
	reason           *string
}

func (uc *UseCase) finalizeCheckoutPaymentAttemptForExpiredContract(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractID model.ContractID,
	now time.Time,
) (paymentAttemptExpiryTransition, error) {
	attempt, err := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx, contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return paymentAttemptExpiryTransition{}, nil
		}

		return paymentAttemptExpiryTransition{}, err
	}

	transition := paymentAttemptExpiryTransition{}
	switch attempt.State {
	case model.CreatedPaymentAttemptState:
		reason := "contract_expired_before_payment_submission"
		if _, err := paymentAttemptQ.MarkCanceledFromCreated(ctx, attempt.ID, reason, now, now); err != nil {
			if cerr.IsCode(err, cerr.CodeConflict) {
				return paymentAttemptExpiryTransition{}, nil
			}

			return paymentAttemptExpiryTransition{}, err
		}

		before, after := attempt.State, model.CanceledPaymentAttemptState
		transition = paymentAttemptExpiryTransition{
			affectedCount:    1,
			paymentAttemptID: &attempt.ID,
			previousState:    &before,
			currentState:     &after,
			reason:           &reason,
		}

	case model.FailedPaymentAttemptState, model.CanceledPaymentAttemptState:

		// Already terminal non-success. No payment-attempt transition is needed;
		// expiring the payment_pending contract only releases the local gate.

	default:
		// paymentPendingContractSafeToExpire has already rejected all ambiguous or
		// successful states while holding the payment-attempt row lock. Reaching
		// another state here indicates a programming/invariant violation.
		return paymentAttemptExpiryTransition{}, cerr.InternalError(
			fmt.Errorf("cannot expire checkout contract with payment attempt state %q", attempt.State),
		)
	}

	return transition, nil
}

// contractRequiresInitialPaymentInTx reads the Decision's Checkout Policy inside
// an open transaction. payment_pending alone never implies a PaymentAttempt.
func (uc *UseCase) contractRequiresInitialPaymentInTx(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	contractRow model.Contract,
) (bool, error) {
	const op = "contractuc.UseCase.contractRequiresInitialPaymentInTx"

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
