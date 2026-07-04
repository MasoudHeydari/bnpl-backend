package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// checkoutSessionCanceledFailureCode records why an unsubmitted checkout payment
// attempt was closed together with its checkout session.
const checkoutSessionCanceledFailureCode = "checkout_session_canceled_before_payment_submission"

// sessionAuthorizer reports whether the caller may act on a locked session.
type sessionAuthorizer func(ctx context.Context, tx repo.Tx, session model.CheckoutSession) error

// CancelByMerchant cancels a checkout the merchant owns.
//
// Cancellation is idempotent: an already-canceled checkout succeeds and returns
// the same return URL. The cancellation reason is merchant_canceled for
// integration events; merchant-facing status stays canceled.
func (uc *UseCase) CancelByMerchant(
	ctx context.Context,
	input CancelByMerchantInput,
) (CancelOutput, error) {
	if err := input.CheckoutSessionToken.Validate(); err != nil {
		return CancelOutput{}, checkoutSessionTokenInvalidError(err)
	}

	if input.MerchantID.IsZero() {
		return CancelOutput{}, merchantIDRequiredError()
	}

	return uc.cancelByPublicToken(
		ctx,
		input.CheckoutSessionToken,
		model.MerchantCanceledCheckoutReason,
		uc.merchantAuthorizer(input.MerchantID),
	)
}

// CancelByToken cancels a checkout abandoned from the hosted page.
//
// FIXME(checkout-cancel-attribution): Possession of the public checkout
// token does not prove the caller is the customer; the merchant also
// receives this token at Create. Therefore customer_canceled is currently
// an inferred channel reason, not authenticated actor attribution.
// Revisit cancellation authorization and audit semantics separately.
func (uc *UseCase) CancelByToken(
	ctx context.Context,
	input CancelByTokenInput,
) (CancelOutput, error) {
	if err := input.Token.Validate(); err != nil {
		return CancelOutput{}, checkoutSessionTokenInvalidError(err)
	}

	return uc.cancelByPublicToken(
		ctx,
		input.Token,
		model.CustomerCanceledCheckoutReason,
		uc.publicTokenAuthorizer(),
	)
}

func (uc *UseCase) publicTokenAuthorizer() sessionAuthorizer {
	const op = "checkoutsessionuc.UseCase.publicTokenAuthorizer"

	return func(_ context.Context, _ repo.Tx, session model.CheckoutSession) error {
		if session.Origin != model.ExternalPartnerCheckoutSessionOrigin {
			return checkoutSessionNotFoundError(
				fmt.Errorf("%s: checkout session is not reachable by public token", op),
			)
		}

		return nil
	}
}

// cancelByPublicToken resolves the merchant-facing public token to the
// internal session id, then cancels under the same transactional path as
// cancel. Resolution and cancellation each lock the row so a concurrent
// state change cannot be missed between the two steps.
func (uc *UseCase) cancelByPublicToken(
	ctx context.Context,
	token model.CheckoutSessionToken,
	reason model.CheckoutCancellationReason,
	authorize sessionAuthorizer,
) (CancelOutput, error) {
	const op = "checkoutsessionuc.UseCase.cancelByPublicToken"

	var id model.CheckoutSessionID

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			sessionQ := uc.checkoutSession.Conn(conn)

			found, findErr := sessionQ.GetByPublicToken(ctx, token)
			if findErr != nil {
				if cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(findErr)
				}

				return findErr
			}

			id = found.ID

			return nil
		},
	)
	if err != nil {
		return CancelOutput{}, err
	}

	if id.IsZero() {
		return CancelOutput{}, fmt.Errorf("%s: resolved checkout session id is zero", op)
	}

	return uc.cancel(ctx, id, reason, authorize)
}

func (uc *UseCase) cancel(
	ctx context.Context,
	id model.CheckoutSessionID,
	reason model.CheckoutCancellationReason,
	authorize sessionAuthorizer,
) (CancelOutput, error) {
	const op = "checkoutsessionuc.UseCase.cancel"

	var (
		session model.CheckoutSession
		expired bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)

					locked, lockErr := sessionQ.LockByID(ctx, id)
					if lockErr != nil {
						if cerr.IsCode(lockErr, cerr.CodeResourceNotFound) {
							return checkoutSessionNotFoundError(lockErr)
						}

						return lockErr
					}

					if authErr := authorize(ctx, tx, locked); authErr != nil {
						return authErr
					}

					now := uc.clock.Now().UTC()

					switch locked.Status {
					case model.CanceledCheckoutSessionStatus:
						session = locked

						return nil

					case model.ConfirmingCheckoutSessionStatus, model.ProcessingCheckoutSessionStatus:
						return checkoutPaymentInProgressError(
							fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
						)

					case model.OpenCheckoutSessionStatus:
						// A retryable payment failure may return to OPEN, but the
						// accepted Contract and possible late FC outcome are still
						// authoritative. Cancellation must not terminate that flow.
						if locked.ContractID != nil {
							return checkoutPaymentInProgressError(fmt.Errorf("%s: contract already accepted", op))
						}

						if locked.IsExpiredAt(now) {
							if _, expireErr := uc.transitionToExpired(ctx, tx, locked, now); expireErr != nil {
								return expireErr
							}

							// Commit the expiry (and its lifecycle event) before
							// rejecting the caller; returning the error from the
							// transaction would roll the expiry back.
							expired = true

							return nil
						}

					default:
						return checkoutSessionNotCancelableError(
							fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
						)
					}

					if contractErr := uc.cancelPaymentPendingContract(ctx, tx, locked, now); contractErr != nil {
						return contractErr
					}

					canceled, statusErr := sessionQ.MarkCanceled(ctx, locked.ID, reason, now)
					if statusErr != nil {
						return statusErr
					}

					if enqueueErr := uc.lifecycle.Enqueue(
						ctx,
						tx,
						canceled,
						checkoutlifecycle.EventTypeCanceled,
						now,
					); enqueueErr != nil {
						return enqueueErr
					}

					session = canceled

					return nil
				},
			)
		},
	)
	if err != nil {
		return CancelOutput{}, err
	}

	if expired {
		return CancelOutput{}, checkoutSessionExpiredError()
	}

	// The return URL is built only after the cancellation commits, so a client
	// that follows it never observes a checkout that is still open. Browser
	// return parameters are UX only — merchants must confirm via status/webhook.
	returnURL, err := merchantReturnURL(session)
	if err != nil {
		return CancelOutput{}, err
	}

	return CancelOutput{Session: session, ReturnURL: returnURL}, nil
}

func (uc *UseCase) merchantAuthorizer(merchantID model.MerchantID) sessionAuthorizer {
	const op = "checkoutsessionuc.UseCase.merchantAuthorizer"

	return func(ctx context.Context, tx repo.Tx, session model.CheckoutSession) error {
		merchantProfileQ := uc.merchantProfile.Tx(tx)

		profile, err := merchantProfileQ.GetByMerchantID(ctx, merchantID)
		if err != nil {
			return err
		}

		if session.MerchantProfileID != profile.ID {
			return checkoutSessionNotFoundError(
				fmt.Errorf("%s: checkout session is owned by another merchant", op),
			)
		}

		return nil
	}
}

func (uc *UseCase) customerAuthorizer(externalCustomerID model.ExternalCustomerID) sessionAuthorizer {
	return func(ctx context.Context, tx repo.Tx, session model.CheckoutSession) error {
		customerProfileQ := uc.customerProfile.Tx(tx)

		profile, err := customerProfileQ.LockByExternalCustomerID(ctx, externalCustomerID)
		if err != nil {
			return err
		}

		// Only the customer already attached to the session (trusted internal
		// customer or Identity-resolved external customer) may abandon it.
		return assertBoundCustomer(session, profile.ID)
	}
}

// cancelPaymentPendingContract closes the pre-activation contract a checkout
// session created, when no payment has left BNPL.
//
// An activated or canceling contract carries real credit exposure and is never
// touched from the checkout boundary; contract cancellation owns that path.
func (uc *UseCase) cancelPaymentPendingContract(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
	now time.Time,
) error {
	const op = "checkoutsessionuc.UseCase.cancelPaymentPendingContract"

	if session.ContractID == nil {
		return nil
	}

	contractQ := uc.contract.Tx(tx)
	paymentAttemptQ := uc.paymentAttempt.Tx(tx)
	installmentQ := uc.installment.Tx(tx)
	installmentChargeQ := uc.installmentCharge.Tx(tx)

	locked, err := contractQ.LockByID(ctx, *session.ContractID)
	if err != nil {
		return err
	}

	switch locked.Status {
	case model.PaymentPendingContractStatus:
		// Pre-activation contracts hold no exposure and may be closed here.

	case model.CanceledContractStatus, model.ExpiredContractStatus:
		return nil

	default:
		return checkoutSessionNotCancelableError(
			fmt.Errorf("%s: contract status is %q", op, locked.Status),
		)
	}

	safe, err := checkoutPaymentSafeToCancel(ctx, paymentAttemptQ, locked.ID)
	if err != nil {
		return err
	}

	if !safe {
		return checkoutPaymentInProgressError(
			fmt.Errorf("%s: checkout payment has an unresolved external outcome", op),
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

// checkoutPaymentSafeToCancel reports whether the checkout payment provably
// never left BNPL or is already terminal without success.
//
// Submitted, reconciliation-required, manual-review, and succeeded attempts have
// an externally observable outcome. Canceling the checkout around them could
// release the single-in-flight-payment gate while Financial Core still reports a
// success, so they block cancellation until resolved.
func checkoutPaymentSafeToCancel(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractID model.ContractID,
) (bool, error) {
	const op = "checkoutsessionuc.checkoutPaymentSafeToCancel"

	attempt, err := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx, contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return true, nil
		}

		return false, err
	}

	// Lock the attempt after resolving it by contract/purpose, matching the
	// contract -> payment attempt lock order used by payment result processing.
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
		return false, cerr.InternalError(
			fmt.Errorf("%s: unknown checkout payment attempt state %q", op, attempt.State),
		)
	}
}

// cancelUnsubmittedCheckoutPayment closes a checkout payment attempt that never
// reached Financial Core.
func cancelUnsubmittedCheckoutPayment(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractID model.ContractID,
	now time.Time,
) error {
	const op = "checkoutsessionuc.cancelUnsubmittedCheckoutPayment"

	attempt, err := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
		ctx, contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		return err
	}

	switch attempt.State {
	case model.CreatedPaymentAttemptState:
		if _, err = paymentAttemptQ.MarkCanceledFromCreated(
			ctx,
			attempt.ID,
			checkoutSessionCanceledFailureCode,
			now,
			now,
		); err != nil {
			return err
		}

	case model.FailedPaymentAttemptState, model.CanceledPaymentAttemptState:
		// Already terminal without success; nothing to transition.

	default:
		// checkoutPaymentSafeToCancel rejected every ambiguous or successful state
		// while holding the attempt lock, so any other state is an invariant break.
		return cerr.InternalError(
			fmt.Errorf("%s: cannot cancel checkout with payment attempt state %q", op, attempt.State),
		)
	}

	return nil
}
