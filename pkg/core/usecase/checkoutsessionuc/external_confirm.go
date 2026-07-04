package checkoutsessionuc

import (
	"context"
	"fmt"
	"strconv"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

// verifiedExternalCheckout is the trusted identity proven by the OTP step.
// Financial Core scope is resolved later at payment submission — never from
// browser JWT headers or fabricated checkout snapshots.
type verifiedExternalCheckout struct {
	sessionID          model.CheckoutSessionID
	externalCustomerID model.ExternalCustomerID
}

// ConfirmExternal verifies a still-unused one-time code for the exact offer
// version the customer saw, then runs the regular Confirm & Pay flow for the
// customer bound to the session.
//
// OTP is strictly single-use: a consumed code never authorizes another public
// Confirm, and otp_verified_at alone never bypasses verification. Internal Saga
// workers recover authorized operations without going through this path.
//
// The browser supplies no JWT tenant/context headers and no payment scope in
// the body. Financial Core scope is resolved only when submitting payment.
// A wrong code is counted atomically and persisted; once the budget is spent
// only a fresh code can unlock verification.
//
// Stale evaluation_generation is rejected before any OTP attempt is consumed so
// a refreshed offer never burns the customer's wrong-code budget.
//
// Continuation confirm acquires locks in the canonical OTP×Saga×FC order
// (Saga → Contract → PaymentAttempt → CheckoutSession) and asserts business
// eligibility before consume so a manual_review/reconciliation race cannot
// burn a valid OTP.
func (uc *UseCase) ConfirmExternal(
	ctx context.Context,
	input ConfirmExternalInput,
) (ConfirmOutput, error) {
	if err := input.validate(); err != nil {
		return ConfirmOutput{}, err
	}

	verified, err := uc.verifyExternalOTP(ctx, input)
	if err != nil {
		return ConfirmOutput{}, err
	}

	return uc.Confirm(ctx, ConfirmInput{
		CheckoutSessionID:    verified.sessionID,
		ExternalCustomerID:   verified.externalCustomerID,
		EvaluationGeneration: input.EvaluationGeneration,
	})
}

// verifyExternalOTP checks the code and records the outcome. A failed attempt
// is committed before the rejection is returned, since returning the error
// from the transaction would roll the attempt counter back.
//
// Business blockers discovered under Saga/Contract/PA locks abort the
// transaction without consuming the challenge (ConsumeAuthorizer / pre-checks).
func (uc *UseCase) verifyExternalOTP(
	ctx context.Context,
	input ConfirmExternalInput,
) (verifiedExternalCheckout, error) {
	const op = "checkoutsessionuc.UseCase.verifyExternalOTP"

	var (
		verified  verifiedExternalCheckout
		rejection error
		expired   bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					customerProfileQ := uc.customerProfile.Tx(tx)

					peeked, err := uc.peekExternalSession(ctx, tx, input.Token)
					if err != nil {
						return err
					}

					now := uc.clock.Now().UTC()

					// Detect a stale offer before consuming OTP attempts.
					if peeked.EvaluationGeneration != input.EvaluationGeneration {
						return checkoutOfferReconfirmRequiredError(
							fmt.Errorf(
								"%s: evaluation generation %d does not match session generation %d",
								op,
								input.EvaluationGeneration,
								peeked.EvaluationGeneration,
							),
						)
					}

					if !allowsExternalConfirmWithOTP(peeked.Status) {
						return checkoutSessionNotContinuableError(
							fmt.Errorf("%s: checkout session status is %q", op, peeked.Status),
						)
					}

					var (
						locked            model.CheckoutSession
						consumeAuthorizer otp.ConsumeAuthorizer
					)

					switch peeked.Status {
					case model.OpenCheckoutSessionStatus:
						// Lock CS under the peeked open path. A concurrent Confirm
						// may have already moved the row to confirming/processing
						// while we waited — then skip Saga→C→PA (we already hold
						// CS) and let consume report already-consumed.
						locked, err = uc.lockExternalSession(ctx, tx, input.Token)
						if err != nil {
							return err
						}

						switch locked.Status {
						case model.OpenCheckoutSessionStatus:
							if locked.ContractID == nil && locked.IsExpiredAt(now) {
								_, err = uc.transitionToExpired(ctx, tx, locked, now)
								if err != nil {
									return err
								}

								expired = true

								return nil
							}

							// Confirm open-path gates: customer binding and lifetime.
							// Financing decision readiness is enforced by Confirm
							// core after a successful consume.
							if locked.CustomerID == nil || locked.CustomerID.IsZero() {
								return checkoutOfferNotReadyError(
									fmt.Errorf("%s: checkout was not started", op),
								)
							}

							if locked.IsExpiredAt(now) {
								return checkoutSessionExpiredError()
							}

						case model.ConfirmingCheckoutSessionStatus, model.ProcessingCheckoutSessionStatus:
							if locked.CustomerID == nil || locked.CustomerID.IsZero() {
								return checkoutOfferNotReadyError(
									fmt.Errorf("%s: customer binding is required", op),
								)
							}

						default:
							return checkoutSessionNotContinuableError(
								fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
							)
						}

					case model.ConfirmingCheckoutSessionStatus, model.ProcessingCheckoutSessionStatus:
						// First-confirm races may observe confirming before ContractID
						// is linked. Only the linked-contract path needs Saga→C→PA
						// continuation gates; otherwise lock CS and let consume
						// report already-consumed / not-requested as appropriate.
						if peeked.ContractID != nil && !peeked.ContractID.IsZero() {
							// Assert eligibility under Saga→Contract→PA BEFORE
							// consume so manual_review cannot burn the OTP.
							if _, err := uc.lockOTPContinuationDepsInTx(ctx, tx, peeked); err != nil {
								return err
							}

							locked, err = uc.lockExternalSession(ctx, tx, input.Token)
							if err != nil {
								return err
							}

							if err := assertContinuationSessionMatchesPeeked(locked, peeked, op); err != nil {
								return err
							}

							if locked.CustomerID == nil || locked.CustomerID.IsZero() {
								return checkoutOTPIssuanceNotAllowedError(
									fmt.Errorf("%s: customer binding is required", op),
								)
							}

							consumeAuthorizer = &continuationOTPConsumeAuthorizer{
								uc:      uc,
								session: locked,
							}
						} else {
							locked, err = uc.lockExternalSession(ctx, tx, input.Token)
							if err != nil {
								return err
							}

							if locked.CustomerID == nil || locked.CustomerID.IsZero() {
								return checkoutOfferNotReadyError(
									fmt.Errorf("%s: customer binding is required", op),
								)
							}
						}

					default:
						return checkoutSessionNotContinuableError(
							fmt.Errorf("%s: checkout session status is %q", op, peeked.Status),
						)
					}

					rejection, err = uc.consumeOTP(ctx, tx, locked, input, consumeAuthorizer)
					if err != nil {
						return err
					}

					if rejection != nil {
						return nil
					}

					profile, err := customerProfileQ.LockByID(ctx, *locked.CustomerID)
					if err != nil {
						return err
					}

					verified = verifiedExternalCheckout{
						sessionID:          locked.ID,
						externalCustomerID: profile.ExternalCustomerID,
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return verifiedExternalCheckout{}, err
	}

	if expired {
		return verifiedExternalCheckout{}, checkoutSessionExpiredError()
	}

	if rejection != nil {
		return verifiedExternalCheckout{}, rejection
	}

	return verified, nil
}

// continuationOTPConsumeAuthorizer re-asserts continuation eligibility under
// locks already held (Saga → Contract → PA → CS) after a correct code hash
// match and before MarkVerified. Failure rolls the consume transaction back so
// the challenge stays usable (for example when Saga entered manual_review).
type continuationOTPConsumeAuthorizer struct {
	uc      *UseCase
	session model.CheckoutSession
}

func (a *continuationOTPConsumeAuthorizer) AuthorizeConsume(
	ctx context.Context,
	tx repo.Tx,
) error {
	// Re-run the same Saga → Contract → PA lock+validate path. Locks are already
	// held by verifyExternalOTP in canonical order; LockByID waits are no-ops.
	_, err := a.uc.lockOTPContinuationDepsInTx(ctx, tx, a.session)

	return err
}

// consumeOTP verifies the presented code via the generic OTP use case under
// the application locks already held by the caller. Wrong codes increment the
// attempt counter; stale context mismatches are rejected without burning an
// attempt. A non-nil ConsumeAuthorizer runs after a correct hash and before
// MarkVerified so business blockers can abort without consuming.
//
// The returned rejection is a business outcome that must still commit (the
// failed attempt is persisted); err aborts and rolls the transaction back.
func (uc *UseCase) consumeOTP(
	ctx context.Context,
	tx repo.Tx,
	locked model.CheckoutSession,
	input ConfirmExternalInput,
	consumeAuthorizer otp.ConsumeAuthorizer,
) (rejection error, err error) {
	const op = "checkoutsessionuc.UseCase.consumeOTP"

	_, rejection, err = uc.otp.VerifyAndConsumeInTx(ctx, tx, otp.VerifyInput{
		Purpose:           model.OTPPurposeHostedCheckoutConfirm,
		SubjectKey:        locked.ID.String(),
		ContextVersion:    strconv.Itoa(input.EvaluationGeneration),
		Binding:           input.Token.String(),
		Code:              input.OTP,
		ConsumeAuthorizer: consumeAuthorizer,
	})
	if err != nil {
		if otp.IsContextInvalidated(err) {
			return nil, checkoutOfferReconfirmRequiredError(
				fmt.Errorf(
					"%s: otp challenge generation does not match evaluation generation %d: %w",
					op,
					locked.EvaluationGeneration,
					err,
				),
			)
		}

		return nil, err
	}

	return rejection, nil
}
