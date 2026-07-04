package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// Confirm accepts the offer version the customer actually saw and starts
// durable confirmation through the checkout confirmation Saga.
//
// Flow:
//  1. Short TX: lock session, validate customer, refuse cancel/expiry races.
//     Matching generation + non-expired Decision is claimed open → confirming
//     atomically with Saga creation after Decision-domain revalidation.
//  2. The Saga prepares the Contract, links it, submits to Financial Core, and
//     finalizes merchant-funding recognition. HTTP Confirm drives ExecuteByID
//     for low latency; saga_executor remains the durable progress owner.
//  3. Unlinked confirming without an owning HTTP request resumes the existing
//     Saga instead of returning permanent IN_PROGRESS with no recovery path.
//  4. Payment activation for initial-payment contracts stays on the
//     authoritative payment-result path after the Saga reaches
//     checkout_payment_submitted.
//
// reconfirm_required means the previously accepted material offer can no longer
// be honored — not merely that available_credit_minor changed as a snapshot.
func (uc *UseCase) Confirm(ctx context.Context, input ConfirmInput) (ConfirmOutput, error) {
	if err := input.validate(); err != nil {
		return ConfirmOutput{}, err
	}

	prepared, err := uc.beginConfirm(ctx, input)
	if err != nil {
		return ConfirmOutput{}, err
	}

	if prepared.ReconfirmRequired {
		return ConfirmOutput{
			ReconfirmRequired: true,
			Session:           prepared.Session,
			Offer:             prepared.Offer,
		}, nil
	}

	if prepared.Replay != nil {
		return *prepared.Replay, nil
	}

	if prepared.ResumeSaga {
		return uc.resumeConfirmSaga(ctx, prepared.Session, input)
	}

	if prepared.SagaID.IsZero() {
		return ConfirmOutput{}, cerr.InternalError(
			fmt.Errorf("checkoutsessionuc.UseCase.Confirm: confirming claim missing saga id"),
		)
	}

	if err := uc.driveConfirmSaga(ctx, prepared.SagaID); err != nil {
		return ConfirmOutput{}, err
	}

	return uc.projectConfirmAfterSaga(ctx, prepared.Session.ID, prepared.SagaID, input)
}

type beginConfirmResult struct {
	Session           model.CheckoutSession
	ReconfirmRequired bool
	Offer             Offer
	Replay            *ConfirmOutput

	// pendingMaterialCheck means generation matched and the Decision has not
	// expired by wall-clock; Confirm must revalidate material terms before
	// claiming confirming.
	pendingMaterialCheck bool

	// forceNewGeneration asks refreshOfferForReconfirm to bump even when the
	// linked Decision has not expired (material term change).
	forceNewGeneration bool

	// ResumeSaga means an unlinked confirming claim already owns a durable Saga.
	ResumeSaga bool

	// SagaID is set when this Confirm atomically started the confirmation Saga.
	SagaID model.SagaID
}

// beginConfirm locks the session and either claims it as confirming, returns an
// idempotent replay, or signals that the customer must reconfirm a new offer.
func (uc *UseCase) beginConfirm(
	ctx context.Context,
	input ConfirmInput,
) (beginConfirmResult, error) {
	const op = "checkoutsessionuc.UseCase.beginConfirm"

	var (
		result  beginConfirmResult
		expired bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)
					customerProfileQ := uc.customerProfile.Tx(tx)

					locked, lockErr := sessionQ.LockByID(ctx, input.CheckoutSessionID)
					if lockErr != nil {
						if cerr.IsCode(lockErr, cerr.CodeResourceNotFound) {
							return checkoutSessionNotFoundError(lockErr)
						}

						return lockErr
					}

					now := uc.clock.Now().UTC()

					if locked.Status == model.OpenCheckoutSessionStatus && locked.ContractID == nil && locked.IsExpiredAt(now) {
						if _, statusErr := uc.transitionToExpired(ctx, tx, locked, now); statusErr != nil {
							return statusErr
						}

						// Commit the expiry (and its lifecycle event) before
						// rejecting the caller; returning the error from the
						// transaction would roll the expiry back.
						expired = true

						return nil
					}

					profile, profileErr := customerProfileQ.LockByExternalCustomerID(ctx, input.ExternalCustomerID)
					if profileErr != nil {
						return profileErr
					}

					if bindingErr := assertBoundCustomer(locked, profile.ID); bindingErr != nil {
						return bindingErr
					}

					switch locked.Status {
					case model.CompletedCheckoutSessionStatus, model.ProcessingCheckoutSessionStatus:
						if locked.ContractID == nil {
							return checkoutSessionNotContinuableError(
								fmt.Errorf("%s: checkout session status is %q without contract", op, locked.Status),
							)
						}

						result.Session = locked

						return nil

					case model.ConfirmingCheckoutSessionStatus:
						if locked.ContractID != nil {
							result.Session = locked

							return nil
						}

						if locked.ConfirmationAttemptID == nil || locked.ConfirmationAttemptID.IsZero() {
							return checkoutPaymentInProgressError(
								fmt.Errorf("%s: confirming session %s has no confirmation attempt", op, locked.ID),
							)
						}

						// An unlinked confirming claim is owned by the durable
						// confirmation Saga. Concurrent callers resume that Saga
						// rather than starting a second prepare.
						result.Session = locked
						result.ResumeSaga = true

						return nil

					case model.OpenCheckoutSessionStatus:
						// An accepted Contract already linked means payment retry
						// (or recovery after a retryable FC failure). Never re-evaluate
						// the offer or clear that linkage because the Decision expired.
						if locked.ContractID != nil {
							result.Session = locked

							return nil
						}

						// Fresh confirm path continues below.

					default:
						return checkoutSessionNotContinuableError(
							fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
						)
					}

					if locked.EvaluationGeneration != input.EvaluationGeneration {
						// Client presented a superseded generation. Check this
						// before Decision presence so a bump that cleared the
						// Decision still returns reconfirm_required instead of
						// a misleading "call start first" conflict.
						result.Session = locked
						result.ReconfirmRequired = true

						return nil
					}

					if locked.DecisionID == nil || locked.DecisionID.IsZero() {
						return checkoutOfferNotReadyError(
							fmt.Errorf("%s: checkout session has no decision; call start first", op),
						)
					}

					decisionQ := uc.decisionRepo.Tx(tx)
					decision, findErr := decisionQ.GetByID(ctx, *locked.DecisionID)
					if findErr != nil {
						return findErr
					}

					if decision.ExpiresAt != nil && !decision.ExpiresAt.After(now) {
						result.Session = locked
						result.ReconfirmRequired = true
						result.forceNewGeneration = true

						return nil
					}

					// Defer MarkConfirming until Decision-domain revalidation
					// proves the persisted material terms are still honorable.
					result.Session = locked
					result.pendingMaterialCheck = true

					return nil
				},
			)
		},
	)
	if err != nil {
		return beginConfirmResult{}, err
	}

	if expired {
		return beginConfirmResult{}, checkoutSessionExpiredError()
	}

	if result.ReconfirmRequired {
		refreshed, offer, refreshErr := uc.refreshOfferForReconfirm(
			ctx,
			result.Session,
			input.ExternalCustomerID,
			result.forceNewGeneration,
			true,
		)
		if refreshErr != nil {
			return beginConfirmResult{}, refreshErr
		}

		result.Session = refreshed
		result.Offer = offer
		result.ReconfirmRequired = true

		return result, nil
	}

	// Idempotent replay paths that already hold a contract. An unlinked
	// confirming claim is owned by another operation, not retried here.
	if result.Session.Status == model.CompletedCheckoutSessionStatus ||
		result.Session.Status == model.ProcessingCheckoutSessionStatus ||
		(result.Session.ContractID != nil &&
			(result.Session.Status == model.ConfirmingCheckoutSessionStatus ||
				result.Session.Status == model.OpenCheckoutSessionStatus)) {
		replay, replayErr := uc.replayConfirm(ctx, result.Session, input)
		if replayErr != nil {
			return beginConfirmResult{}, replayErr
		}

		result.Replay = &replay

		return result, nil
	}

	if result.pendingMaterialCheck {
		return uc.claimConfirmingAfterRevalidation(ctx, result.Session, input)
	}

	// Unlinked confirming is owned by the durable Saga (or is an orphan that
	// resumeConfirmSaga fails closed on). Do not reclassify it as offer-not-ready
	// merely because Decision linkage is missing on a corrupted row.
	if result.ResumeSaga {
		return result, nil
	}

	if result.Session.DecisionID == nil || result.Session.DecisionID.IsZero() {
		return beginConfirmResult{}, checkoutOfferNotReadyError(
			fmt.Errorf("%s: confirming session has no decision", op),
		)
	}

	return result, nil
}

// claimConfirmingAfterRevalidation runs Decision-domain honorability check
// outside the claim TX, then either refreshes for reconfirm or MarkConfirming.
func (uc *UseCase) claimConfirmingAfterRevalidation(
	ctx context.Context,
	session model.CheckoutSession,
	input ConfirmInput,
) (beginConfirmResult, error) {
	const op = "checkoutsessionuc.UseCase.claimConfirmingAfterRevalidation"

	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return beginConfirmResult{}, checkoutOfferNotReadyError(
			fmt.Errorf("%s: checkout session has no decision", op),
		)
	}

	revalidation, err := uc.decision.RevalidateCheckoutOffer(ctx, decisionuc.RevalidateCheckoutOfferInput{
		DecisionID: *session.DecisionID,
	})
	if err != nil {
		return beginConfirmResult{}, fmt.Errorf("%s: revalidate checkout offer: %w", op, err)
	}

	if !revalidation.StillHonorable {
		refreshed, offer, refreshErr := uc.refreshOfferForReconfirm(
			ctx,
			session,
			input.ExternalCustomerID,
			true,
			true,
		)
		if refreshErr != nil {
			return beginConfirmResult{}, refreshErr
		}

		return beginConfirmResult{
			Session:           refreshed,
			Offer:             offer,
			ReconfirmRequired: true,
		}, nil
	}

	return uc.claimConfirmingWithSaga(ctx, session)
}

// claimConfirmingWithSaga claims open → confirming and creates the confirmation
// Saga in one transaction. Concurrent winners resume the existing Saga.
func (uc *UseCase) claimConfirmingWithSaga(
	ctx context.Context,
	session model.CheckoutSession,
) (beginConfirmResult, error) {
	const op = "checkoutsessionuc.UseCase.claimConfirmingWithSaga"

	var (
		confirming model.CheckoutSession
		sagaID     model.SagaID
		expired    bool
		resume     bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)

			locked, lockErr := sessionQ.LockByID(ctx, session.ID)
			if lockErr != nil {
				return lockErr
			}

			now := uc.clock.Now().UTC()

			if locked.Status == model.ConfirmingCheckoutSessionStatus && locked.ContractID == nil {
				if locked.ConfirmationAttemptID == nil || locked.ConfirmationAttemptID.IsZero() {
					return checkoutPaymentInProgressError(
						fmt.Errorf("%s: confirming session %s has no confirmation attempt", op, locked.ID),
					)
				}

				confirming = locked
				resume = true
				return nil
			}

			if locked.Status != model.OpenCheckoutSessionStatus || locked.ContractID != nil {
				return checkoutPaymentInProgressError(
					fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
				)
			}

			// Revalidation runs outside this transaction. Re-check the absolute
			// checkout deadline under the row lock so a session cannot enter
			// confirming after expires_at elapsed during revalidation.
			if locked.IsExpiredAt(now) {
				transitioned, statusErr := uc.transitionToExpired(ctx, tx, locked, now)
				if statusErr != nil {
					return statusErr
				}

				confirming = transitioned
				expired = true
				return nil
			}

			if locked.EvaluationGeneration != session.EvaluationGeneration ||
				!sameDecisionLink(locked.DecisionID, session.DecisionID) {
				return checkoutOfferReconfirmRequiredError(
					fmt.Errorf("%s: evaluation state changed before confirming claim", op),
				)
			}

			claimed, startedSagaID, startErr := uc.startConfirmSagaTx(ctx, tx, locked, now)
			if startErr != nil {
				return startErr
			}

			confirming = claimed
			sagaID = startedSagaID
			return nil
		})
	})
	if err != nil {
		return beginConfirmResult{}, err
	}

	if expired {
		return beginConfirmResult{}, checkoutSessionExpiredError()
	}

	if resume {
		return beginConfirmResult{Session: confirming, ResumeSaga: true}, nil
	}

	return beginConfirmResult{Session: confirming, SagaID: sagaID}, nil
}

// markConfirmingOpenSession is retained for focused unit tests of the claim
// deadline race. Production Confirm uses claimConfirmingWithSaga.
func (uc *UseCase) markConfirmingOpenSession(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	result, err := uc.claimConfirmingWithSaga(ctx, session)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	return result.Session, nil
}

// refreshOfferForReconfirm bumps evaluation generation when required, creates a
// fresh Decision through the normal Create path, and returns the new offer.
//
// forceNewGeneration must be true when the existing Decision has not expired
// but its material terms can no longer be honored. When false, a bump occurs
// only if prepareEvaluationGeneration detects Decision expiry (client presented
// a stale generation against a still-valid current Decision merely replays it).
//
// When evaluateNow is false (post-compensation recovery), only the superseded
// Decision/generation is cleared. Evaluating immediately can terminalize the
// session as unavailable while credit is still zero; unavailable is permanent,
// but a compensated confirmation must leave the checkout confirmable so a later
// Start/Confirm can succeed after conditions recover.
//
// When evaluateNow is true, a denied fresh evaluation transitions the session
// to unavailable and returns checkoutOfferUnavailableError — never
// reconfirm_required with a denied offer.
//
// Reconfirmation never extends CheckoutSession.expires_at (ClampExpiresAt is LEAST).
func (uc *UseCase) refreshOfferForReconfirm(
	ctx context.Context,
	session model.CheckoutSession,
	externalCustomerID model.ExternalCustomerID,
	forceNewGeneration bool,
	evaluateNow bool,
) (model.CheckoutSession, Offer, error) {
	const op = "checkoutsessionuc.UseCase.refreshOfferForReconfirm"

	var (
		bumped model.CheckoutSession
		err    error
	)

	if forceNewGeneration {
		bumped, err = uc.forceNewEvaluationGeneration(ctx, session)
	} else {
		bumped, err = uc.prepareEvaluationGeneration(ctx, session)
	}

	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	if !evaluateNow {
		return bumped, Offer{}, nil
	}

	evaluated, offer, err := uc.evaluateSession(ctx, bumped, externalCustomerID)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	if evaluated.Status == model.UnavailableCheckoutSessionStatus ||
		offer.Outcome != ApprovedOfferOutcome {
		return evaluated, Offer{Outcome: DeniedOfferOutcome}, checkoutOfferUnavailableError(
			fmt.Errorf("%s: refreshed evaluation is not approved", op),
		)
	}

	// Shorten only; never extend the absolute checkout deadline.
	clamped, clampErr := uc.clampExternalSessionToOffer(ctx, evaluated, offer)
	if clampErr != nil {
		return model.CheckoutSession{}, Offer{}, clampErr
	}

	return clamped, offer, nil
}

// forceNewEvaluationGeneration bumps evaluation_generation and clears OTP /
// Decision linkage so the next Create uses a fresh CheckoutKey. Concurrent CAS
// winners are reused without creating a second competing generation.
func (uc *UseCase) forceNewEvaluationGeneration(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return session, nil
	}

	if session.ContractID != nil && !session.ContractID.IsZero() {
		return session, nil
	}

	return uc.bumpEvaluationGenerationCAS(ctx, session)
}

// refreshAfterDeterministicContractReject ensures the checkout is confirmable
// after PrepareCheckoutContract rejected with a known deterministic
// pre-contract rejection (insufficient credit, unusable credit account, or
// Decision/allocation no longer live), then refreshes the offer for reconfirm.
//
// Claim compensation may already have reopened confirming → open and cleared
// the active confirmation_attempt_id; this path must not require the session to
// still be confirming. A temporary financing denial after compensation must
// leave the session open (confirmable) rather than terminal unavailable —
// credit and eligibility can recover for a later confirmation attempt.
// Ambiguous failures must not call this.
func (uc *UseCase) refreshAfterDeterministicContractReject(
	ctx context.Context,
	session model.CheckoutSession,
	input ConfirmInput,
) (ConfirmOutput, error) {
	openSession, err := uc.ensureOpenAfterCompensatedConfirmation(ctx, session)
	if err != nil {
		return ConfirmOutput{}, err
	}

	// Do not evaluate while the condition that caused compensation (for example
	// zero available credit) may still hold — unavailable is terminal.
	refreshed, _, refreshErr := uc.refreshOfferForReconfirm(
		ctx,
		openSession,
		input.ExternalCustomerID,
		true,
		false,
	)
	if refreshErr != nil {
		return ConfirmOutput{}, refreshErr
	}

	return ConfirmOutput{
		ReconfirmRequired: true,
		Session:           refreshed,
	}, nil
}

// ensureOpenAfterCompensatedConfirmation returns an open, unlinked session after
// a compensated confirmation attempt. Compensation may already have reopened
// the row; otherwise this reopens confirming → open under the attempt fence.
func (uc *UseCase) ensureOpenAfterCompensatedConfirmation(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.ensureOpenAfterCompensatedConfirmation"

	switch session.Status {
	case model.OpenCheckoutSessionStatus:
		if session.ContractID != nil && !session.ContractID.IsZero() {
			return model.CheckoutSession{}, checkoutPaymentInProgressError(
				fmt.Errorf("%s: open session %s already has a contract", op, session.ID),
			)
		}

		return session, nil

	case model.ConfirmingCheckoutSessionStatus:
		return uc.reopenUnlinkedConfirming(ctx, session)

	default:
		return model.CheckoutSession{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: checkout session status is %q", op, session.Status),
		)
	}
}

// reopenUnlinkedConfirming returns confirming → open only when durable state
// still proves no Contract is linked to this checkout (and none exists for the
// Decision). Otherwise it fails closed without reopening.
func (uc *UseCase) reopenUnlinkedConfirming(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.reopenUnlinkedConfirming"

	var (
		reopened model.CheckoutSession
		expired  bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)
			contractQ := uc.contract.Tx(tx)

			locked, lockErr := sessionQ.LockByID(ctx, session.ID)
			if lockErr != nil {
				return lockErr
			}

			now := uc.clock.Now().UTC()

			if locked.ContractID != nil {
				return checkoutPaymentInProgressError(
					fmt.Errorf("%s: contract already linked; cannot reopen confirming", op),
				)
			}

			if locked.Status != model.ConfirmingCheckoutSessionStatus {
				return checkoutPaymentInProgressError(
					fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
				)
			}

			if locked.DecisionID != nil && !locked.DecisionID.IsZero() {
				_, contractErr := contractQ.GetByDecisionID(ctx, *locked.DecisionID)
				if contractErr == nil {
					return checkoutPaymentInProgressError(
						fmt.Errorf("%s: contract exists for decision; cannot reopen confirming", op),
					)
				}

				if !cerr.IsCode(contractErr, cerr.CodeResourceNotFound) &&
					!cerr.IsCode(contractErr, cerr.CodeContractNotFound) {
					return contractErr
				}
			}

			if locked.ConfirmationAttemptID == nil || locked.ConfirmationAttemptID.IsZero() {
				return checkoutPaymentInProgressError(
					fmt.Errorf("%s: confirming session %s has no confirmation attempt", op, locked.ID),
				)
			}

			updated, statusErr := sessionQ.ReopenConfirmingToOpen(
				ctx,
				locked.ID,
				*locked.ConfirmationAttemptID,
				now,
			)
			if statusErr != nil {
				return statusErr
			}

			// Hard checkout deadline wins in the same transaction: do not
			// return a usable open session or create a new generation.
			if updated.IsExpiredAt(now) {
				transitioned, expireErr := uc.transitionToExpired(ctx, tx, updated, now)
				if expireErr != nil {
					return expireErr
				}

				reopened = transitioned
				expired = true
				return nil
			}

			reopened = updated
			return nil
		})
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if expired {
		return model.CheckoutSession{}, checkoutSessionExpiredError()
	}

	return reopened, nil
}

func (uc *UseCase) bumpEvaluationGenerationCAS(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.bumpEvaluationGenerationCAS"

	var (
		out     model.CheckoutSession
		expired bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)

					locked, lockErr := sessionQ.LockByID(ctx, session.ID)
					if lockErr != nil {
						return lockErr
					}

					now := uc.clock.Now().UTC()

					if locked.Status == model.OpenCheckoutSessionStatus &&
						locked.ContractID == nil &&
						locked.IsExpiredAt(now) {
						transitioned, statusErr := uc.transitionToExpired(ctx, tx, locked, now)
						if statusErr != nil {
							return statusErr
						}

						out = transitioned
						expired = true
						return nil
					}

					if locked.EvaluationGeneration != session.EvaluationGeneration ||
						!sameDecisionLink(locked.DecisionID, session.DecisionID) {
						out = locked
						return nil
					}

					bumped, bumpErr := sessionQ.BumpEvaluationGenerationCAS(
						ctx,
						locked.ID,
						session.EvaluationGeneration,
						session.DecisionID,
						now,
					)
					if bumpErr == nil {
						out = bumped
						return nil
					}

					if cerr.IsCode(bumpErr, cerr.CodeConflict) || cerr.IsCode(bumpErr, cerr.CodeResourceNotFound) {
						reloaded, reloadErr := sessionQ.LockByID(ctx, session.ID)
						if reloadErr != nil {
							return reloadErr
						}

						out = reloaded
						return nil
					}

					return fmt.Errorf("%s: bump evaluation generation: %w", op, bumpErr)
				},
			)
		},
	)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if expired {
		return model.CheckoutSession{}, checkoutSessionExpiredError()
	}

	return out, nil
}

func (uc *UseCase) replayConfirm(
	ctx context.Context,
	session model.CheckoutSession,
	input ConfirmInput,
) (ConfirmOutput, error) {
	const op = "checkoutsessionuc.UseCase.replayConfirm"

	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return ConfirmOutput{}, checkoutOfferNotReadyError(
			fmt.Errorf("%s: session has no decision for replay", op),
		)
	}

	if session.ContractID == nil {
		return ConfirmOutput{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: contract linkage pending recovery", op),
		)
	}

	var contract model.Contract

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			contractQ := uc.contract.Conn(conn)

			var loadErr error
			contract, loadErr = contractQ.GetByID(ctx, *session.ContractID)

			return loadErr
		},
	)
	if err != nil {
		return ConfirmOutput{}, err
	}

	// Terminal success is read-only: never re-enter payment submission.
	if session.Status == model.CompletedCheckoutSessionStatus {
		return ConfirmOutput{
			Session:         session,
			Contract:        contract,
			PaymentRequired: false,
			Payment:         contractuc.CheckoutPaymentProjection(contract),
		}, nil
	}

	// payment_pending alone does not mean a PaymentAttempt exists. Whether
	// customer payment is required comes from the immutable Decision policy.
	paymentRequired, modeErr := uc.contractRequiresInitialPayment(ctx, contract)
	if modeErr != nil {
		return ConfirmOutput{}, modeErr
	}

	// An already-active initial-payment contract has finished customer payment.
	// Do not re-enter payment submission or mistake it for merchant funding.
	if paymentRequired && contract.Status == model.ActiveContractStatus {
		return ConfirmOutput{
			Session:         session,
			Contract:        contract,
			PaymentRequired: false,
			Payment:         contractuc.CheckoutPaymentProjection(contract),
		}, nil
	}

	prepared := contractuc.CreateContractOutput{
		Contract:        contract,
		PaymentRequired: paymentRequired,
	}

	if paymentRequired {
		attempt, attemptErr := uc.loadCheckoutPaymentAttempt(ctx, contract.ID)
		if attemptErr != nil {
			return ConfirmOutput{}, attemptErr
		}

		prepared.PaymentAttempt = attempt
	}

	// confirming+contract means LinkContract may have committed before status
	// advanced or FC submit. Finish the durable linkage first.
	if session.Status == model.ConfirmingCheckoutSessionStatus {
		linked, linkErr := uc.finishConfirm(ctx, session, prepared)
		if linkErr != nil {
			return ConfirmOutput{}, linkErr
		}

		session = linked
	}

	if paymentRequired {
		if paused, pauseErr := uc.externalFulfillmentPausedBySaga(ctx, session); pauseErr != nil {
			return ConfirmOutput{}, pauseErr
		} else if paused {
			return ConfirmOutput{}, checkoutPaymentInProgressError(
				fmt.Errorf("%s: confirmation saga is waiting before another financial core submission", op),
			)
		}

		submitted, submitErr := uc.contractUC.SubmitPreparedCheckoutFulfillment(ctx, prepared)
		if submitErr != nil {
			return ConfirmOutput{}, submitErr
		}

		awaiting, awaitErr := uc.markOpenCheckoutAwaitingPaymentResult(ctx, session)
		if awaitErr != nil {
			return ConfirmOutput{}, awaitErr
		}

		return ConfirmOutput{
			Session:         awaiting,
			Contract:        submitted.Contract,
			PaymentRequired: true,
			PaymentAttempt:  submitted.PaymentAttempt,
			Payment:         submitted.Payment,
		}, nil
	}

	// An ACTIVE payment-based contract is never an instruction to issue a
	// merchant entitlement. Only the immutable Checkout Policy can authorize
	// merchant-funding fulfillment; an FC payment-result consumer owns normal
	// completion.
	merchantFunding, modeErr := uc.isMerchantFundingContract(ctx, contract)
	if modeErr != nil {
		return ConfirmOutput{}, modeErr
	}

	if !merchantFunding {
		return ConfirmOutput{
			Session: session, Contract: contract,
			PaymentRequired: false, Payment: contractuc.CheckoutPaymentProjection(contract),
		}, nil
	}

	if contract.Status != model.PaymentPendingContractStatus && contract.Status != model.ActiveContractStatus {
		return ConfirmOutput{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: merchant-funding contract status is %q", op, contract.Status),
		)
	}

	// If both the activation and its authoritative FC identifiers were already
	// persisted, finish the checkout locally without issuing another FC request.
	if contract.Status == model.ActiveContractStatus && hasFinancialRecognition(contract) {
		completed, completeErr := uc.completeRecognizedCheckout(ctx, contract)
		if completeErr != nil {
			return ConfirmOutput{}, completeErr
		}

		return ConfirmOutput{Session: completed, Contract: contract, PaymentRequired: false}, nil
	}

	// An unrecognized merchant-funding activation resumes with its stable FC source
	// reference; an unknown outcome must never turn into premature completion.
	if paused, pauseErr := uc.externalFulfillmentPausedBySaga(ctx, session); pauseErr != nil {
		return ConfirmOutput{}, pauseErr
	} else if paused {
		return ConfirmOutput{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: confirmation saga is waiting before another financial core submission", op),
		)
	}

	submitted, submitErr := uc.contractUC.SubmitPreparedCheckoutFulfillment(ctx, prepared)
	if submitErr != nil {
		return ConfirmOutput{}, submitErr
	}

	completed, completeErr := uc.completeRecognizedCheckout(ctx, submitted.Contract)
	if completeErr != nil {
		return ConfirmOutput{}, completeErr
	}

	return ConfirmOutput{
		Session:         completed,
		Contract:        submitted.Contract,
		PaymentRequired: false,
		Payment:         contractuc.CheckoutPaymentProjection(submitted.Contract),
	}, nil
}

// isMerchantFundingContract reports whether the Decision's Checkout Policy
// activates via Financial Core merchant funding (no customer PaymentAttempt).
// A terminal payment result is never mistaken for that mode merely because the
// Contract is already active.
func (uc *UseCase) isMerchantFundingContract(ctx context.Context, contract model.Contract) (bool, error) {
	requiresPayment, err := uc.contractRequiresInitialPayment(ctx, contract)
	if err != nil {
		return false, err
	}

	return !requiresPayment, nil
}

// contractRequiresInitialPayment derives whether customer cash collection is
// required from the immutable Decision policy. Do not infer this from
// payment_pending alone: merchant-funding contracts also use that status with
// no PaymentAttempt.
func (uc *UseCase) contractRequiresInitialPayment(ctx context.Context, contract model.Contract) (bool, error) {
	var ref policy.Ref
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		decisionQ := uc.decisionRepo.Conn(conn)
		decision, getErr := decisionQ.GetByID(ctx, contract.DecisionID)
		if getErr != nil {
			return getErr
		}

		ref = policy.Ref{ID: decision.PolicyID, Version: decision.PolicyVersion}

		return nil
	})
	if err != nil {
		return false, err
	}

	selected, err := uc.checkoutPolicies.Resolve(ref)
	if err != nil {
		return false, err
	}

	return selected.Terms().ActivationMode == checkout.ActivationAfterInitialPayment, nil
}

// hasFinancialRecognition checks the durable identifiers supplied by Financial
// Core. Their presence alone is not permission to change activation mode.
func hasFinancialRecognition(contract model.Contract) bool {
	return contract.FinancialMerchantEntitlementID != nil &&
		contract.FinancialReceivableID != nil &&
		*contract.FinancialMerchantEntitlementID != "" &&
		*contract.FinancialReceivableID != ""
}

// completeRecognizedCheckout verifies durable contract recognition before
// changing merchant-visible checkout state. Contract activation and this
// transition are separate short transactions to keep FC calls outside DB locks.
// Replaying completion is idempotent because ApplyPaymentSucceeded locks the
// session and emits a completed event only for a real state transition.
func (uc *UseCase) completeRecognizedCheckout(
	ctx context.Context,
	contract model.Contract,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.completeRecognizedCheckout"

	if contract.Status != model.ActiveContractStatus ||
		contract.FinancialMerchantEntitlementID == nil ||
		contract.FinancialReceivableID == nil ||
		*contract.FinancialMerchantEntitlementID == "" ||
		*contract.FinancialReceivableID == "" {
		return model.CheckoutSession{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: financial core recognition is not durably recorded", op),
		)
	}

	var completed model.CheckoutSession
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			contractQ := uc.contract.Tx(tx)
			current, loadErr := contractQ.GetByID(ctx, contract.ID)
			if loadErr != nil {
				return loadErr
			}

			if current.Status != model.ActiveContractStatus ||
				current.FinancialMerchantEntitlementID == nil ||
				current.FinancialReceivableID == nil ||
				*current.FinancialMerchantEntitlementID == "" ||
				*current.FinancialReceivableID == "" {
				return checkoutPaymentInProgressError(fmt.Errorf("%s: contract not authoritatively activated", op))
			}

			if err := uc.ApplyPaymentSucceeded(ctx, tx, current.ID, uc.clock.Now().UTC()); err != nil {
				return err
			}

			checkoutSessionQ := uc.checkoutSession.Tx(tx)
			var findErr error
			completed, findErr = checkoutSessionQ.LockByContractID(ctx, current.ID)
			return findErr
		})
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if completed.Status != model.CompletedCheckoutSessionStatus {
		return model.CheckoutSession{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: checkout is not completed", op),
		)
	}

	return completed, nil
}

func (uc *UseCase) loadCheckoutPaymentAttempt(
	ctx context.Context,
	contractID model.ContractID,
) (model.PaymentAttempt, error) {
	const op = "checkoutsessionuc.UseCase.loadCheckoutPaymentAttempt"

	var attempt model.PaymentAttempt

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					paymentAttemptQ := uc.paymentAttempt.Tx(tx)

					var loadErr error
					attempt, loadErr = paymentAttemptQ.GetByContractIDAndPurposeWithItems(
						ctx,
						contractID,
						model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
					)

					return loadErr
				},
			)
		},
	)
	if err != nil {
		return model.PaymentAttempt{}, fmt.Errorf("%s: load checkout payment attempt: %w", op, err)
	}

	return attempt, nil
}

// finishConfirm attaches the contract and advances confirming → processing.
// Completion is reserved for an authoritative FC outcome, even for trusted users.
func (uc *UseCase) finishConfirm(
	ctx context.Context,
	session model.CheckoutSession,
	created contractuc.CreateContractOutput,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.finishConfirm"

	var linked model.CheckoutSession

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)

					locked, lockErr := sessionQ.LockByID(ctx, session.ID)
					if lockErr != nil {
						return lockErr
					}

					now := uc.clock.Now().UTC()

					attached := locked
					alreadyLinked := locked.ContractID != nil && *locked.ContractID == created.Contract.ID

					if !alreadyLinked {
						if locked.Status != model.ConfirmingCheckoutSessionStatus &&
							locked.Status != model.ProcessingCheckoutSessionStatus &&
							locked.Status != model.CompletedCheckoutSessionStatus {
							return checkoutSessionNotContinuableError(
								fmt.Errorf("%s: session status is %q after create contract", op, locked.Status),
							)
						}

						var linkErr error
						attached, linkErr = sessionQ.LinkContract(ctx, locked.ID, created.Contract.ID, now)
						if linkErr != nil {
							return linkErr
						}
					}

					// Both initial-payment and merchant-funding contracts must
					// await authoritative FC success. The absence of a PSP
					// redirect does NOT mean the purchase is completed.
					nextStatus := model.ProcessingCheckoutSessionStatus

					if attached.Status == nextStatus {
						linked = attached

						return nil
					}

					if transitionErr := attached.Status.ValidateTransitionTo(nextStatus); transitionErr != nil {
						return cerr.InternalError(
							fmt.Errorf("%s: transition to %s: %w", op, nextStatus, transitionErr),
						)
					}

					updated, statusErr := sessionQ.UpdateStatus(ctx, attached.ID, nextStatus, now)
					if statusErr != nil {
						return statusErr
					}

					linked = updated

					return nil
				},
			)
		},
	)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	return linked, nil
}

// ApplyPaymentSucceeded marks the checkout completed after Financial Core
// authoritatively activated the linked contract.
//
// Late success against a terminal non-completed session is a reconciliation
// conflict: evidence is recorded and the error surfaces for operators. Success
// never silently disappears.
func (uc *UseCase) ApplyPaymentSucceeded(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	const op = "checkoutsessionuc.UseCase.ApplyPaymentSucceeded"

	sessionQ := uc.checkoutSession.Tx(tx)

	locked, err := sessionQ.LockByContractID(ctx, contractID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		return fmt.Errorf("%s: lock checkout session by contract: %w", op, err)
	}

	switch locked.Status {
	case model.CompletedCheckoutSessionStatus:
		return nil

	case model.ProcessingCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.OpenCheckoutSessionStatus:
		// Normal and recoverable paths into completed.

	default:
		return uc.recordCheckoutPaymentStateConflict(
			ctx,
			tx,
			locked,
			contractID,
			now,
		)
	}

	if transitionErr := locked.Status.ValidateTransitionTo(model.CompletedCheckoutSessionStatus); transitionErr != nil {
		return uc.recordCheckoutPaymentStateConflict(
			ctx,
			tx,
			locked,
			contractID,
			now,
		)
	}

	updated, err := sessionQ.UpdateStatus(ctx, locked.ID, model.CompletedCheckoutSessionStatus, now)
	if err != nil {
		return err
	}

	return uc.lifecycle.Enqueue(
		ctx,
		tx,
		updated,
		checkoutlifecycle.EventTypeCompleted,
		now,
	)
}

// ApplyPaymentRetryableFailure returns a processing checkout to open after a
// terminal FAILED/CANCELED payment attempt while keeping the same Decision and
// payment_pending Contract so Confirm can mint a new PaymentAttempt.
func (uc *UseCase) ApplyPaymentRetryableFailure(
	ctx context.Context,
	tx repo.Tx,
	contractID model.ContractID,
	now time.Time,
) error {
	const op = "checkoutsessionuc.UseCase.ApplyPaymentRetryableFailure"

	sessionQ := uc.checkoutSession.Tx(tx)

	locked, err := sessionQ.LockByContractID(ctx, contractID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		return err
	}

	switch locked.Status {
	case model.CompletedCheckoutSessionStatus:
		// Authoritative success already won; stale failure must not regress.
		return nil

	case model.ProcessingCheckoutSessionStatus:
		// continue

	default:
		return nil
	}

	if transitionErr := locked.Status.ValidateTransitionTo(model.OpenCheckoutSessionStatus); transitionErr != nil {
		return cerr.InternalError(fmt.Errorf("%s: %w", op, transitionErr))
	}

	_, err = sessionQ.UpdateStatus(ctx, locked.ID, model.OpenCheckoutSessionStatus, now)

	return err
}

func (uc *UseCase) recordCheckoutPaymentStateConflict(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
	contractID model.ContractID,
	now time.Time,
) error {
	const op = "checkoutsessionuc.UseCase.recordCheckoutPaymentStateConflict"

	outboxQ := uc.lifecycle.outbox.Tx(tx)

	payload := []byte(fmt.Sprintf(
		`{"code":"CHECKOUT_PAYMENT_STATE_CONFLICT","checkout_session_id":%q,"contract_id":%q,"status":%q,"occurred_at":%q}`,
		session.ID.String(),
		contractID.String(),
		session.Status,
		now.UTC().Format(time.RFC3339Nano),
	))

	aggregateType := checkoutlifecycle.AggregateType
	eventType := "bnpl.checkout.payment_state_conflict.v1"

	_, err := outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.lifecycle.destination,
		MessageKey:    session.ID.String(),
		EventType:     eventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: enqueue conflict evidence: %w", op, err)
	}

	// Returning an error here rolls back the authoritative payment result
	// AND the conflict evidence. Commit both, and let the outbox alert
	// reconciliation/operations to the persisted mismatch.
	return nil
}

// markOpenCheckoutAwaitingPaymentResult moves a linked open checkout back to
// processing after a replacement payment has been submitted. Open cannot stay
// open while an authoritative result is outstanding: cancel and expiry would
// otherwise race the payment, and success could not complete the session.
func (uc *UseCase) markOpenCheckoutAwaitingPaymentResult(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.markOpenCheckoutAwaitingPaymentResult"

	if session.Status != model.OpenCheckoutSessionStatus || session.ContractID == nil || session.ContractID.IsZero() {
		return session, nil
	}

	var updated model.CheckoutSession
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)

			locked, lockErr := sessionQ.LockByID(ctx, session.ID)
			if lockErr != nil {
				return lockErr
			}

			if locked.Status != model.OpenCheckoutSessionStatus ||
				locked.ContractID == nil ||
				*locked.ContractID != *session.ContractID {
				updated = locked

				return nil
			}

			now := uc.clock.Now().UTC()
			if transitionErr := locked.Status.ValidateTransitionTo(model.ProcessingCheckoutSessionStatus); transitionErr != nil {
				return cerr.InternalError(fmt.Errorf("%s: %w", op, transitionErr))
			}

			row, statusErr := sessionQ.UpdateStatus(ctx, locked.ID, model.ProcessingCheckoutSessionStatus, now)
			if statusErr != nil {
				return statusErr
			}

			updated = row

			return nil
		})
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	return updated, nil
}

// externalFulfillmentPausedBySaga reports whether the confirmation Saga has
// scheduled a retry or stopped for reconciliation/manual review. Replay must
// not send the same unchanged Financial Core operation in that interval.
func (uc *UseCase) externalFulfillmentPausedBySaga(
	ctx context.Context,
	session model.CheckoutSession,
) (bool, error) {
	if session.ConfirmationAttemptID == nil || session.ConfirmationAttemptID.IsZero() {
		return false, nil
	}

	instance, err := uc.findConfirmationSagaBySession(ctx, session)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return false, nil
		}

		return false, err
	}

	switch instance.Status {
	case model.ReconciliationRequiredSagaStatus,
		model.ManualReviewRequiredSagaStatus,
		model.FailedSagaStatus:
		return true, nil

	case model.RunningSagaStatus:
		if instance.NextAttemptAt != nil && instance.NextAttemptAt.After(uc.clock.Now().UTC()) {
			return true, nil
		}
	}

	return false, nil
}

// sameDecisionLink compares nullable Decision IDs without comparing addresses.
func sameDecisionLink(a, b *model.DecisionID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return *a == *b
}
