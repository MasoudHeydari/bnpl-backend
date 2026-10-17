package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/checkoutotp"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// verifiedExternalCheckout is the trusted identity proven by the OTP step.
// Financial Core scope is resolved later at payment submission — never from
// browser JWT headers or fabricated checkout snapshots.
type verifiedExternalCheckout struct {
	sessionID         model.CheckoutSessionID
	identitySubjectID model.IdentitySubjectID
}

// ConfirmExternal verifies the one-time code for the exact offer version the
// customer saw, then runs the regular Confirm & Pay flow for the customer bound
// to the session.
//
// The Confirm core receives the customer from the session binding and the
// The browser supplies no JWT tenant/context headers and no payment scope in
// the body. Financial Core scope is resolved only when submitting payment.
// A session whose phone was already verified for the same evaluation generation
// is not re-challenged, so a payment retry after a retryable failure does not
// need a new code. A wrong code is counted atomically and persisted; once the
// budget is spent only a fresh code can unlock verification.
//
// Stale evaluation_generation is rejected before any OTP attempt is consumed so
// a refreshed offer never burns the customer's wrong-code budget.
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
		IdentitySubjectID:    verified.identitySubjectID,
		EvaluationGeneration: input.EvaluationGeneration,
	})
}

// verifyExternalOTP checks the code and records the outcome. A failed attempt
// is committed before the rejection is returned, since returning the error
// from the transaction would roll the attempt counter back.
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
					sessionQ := uc.checkoutSession.Tx(tx)
					customerProfileQ := uc.customerProfile.Tx(tx)

					locked, err := uc.lockExternalSession(ctx, tx, input.Token)
					if err != nil {
						return err
					}

					now := uc.clock.Now().UTC()

					if locked.CustomerID == nil {
						return checkoutOfferNotReadyError(
							fmt.Errorf("%s: checkout was not started", op),
						)
					}

					// Detect a stale offer before consuming OTP attempts.
					if locked.EvaluationGeneration != input.EvaluationGeneration {
						return checkoutOfferReconfirmRequiredError(
							fmt.Errorf(
								"%s: evaluation generation %d does not match session generation %d",
								op,
								input.EvaluationGeneration,
								locked.EvaluationGeneration,
							),
						)
					}

					// Prior verification only authorizes the same evaluation
					// generation. A bumped generation clears OTPVerifiedAt in
					// BumpEvaluationGenerationCAS; this guard covers any race
					// or leftover binding.
					alreadyVerified := locked.OTPVerifiedAt != nil &&
						locked.OTPEvaluationGeneration != nil &&
						*locked.OTPEvaluationGeneration == locked.EvaluationGeneration

					if alreadyVerified {
						// Idempotent Confirm retry after a lost response: OTP was
						// already proven for these offer terms. Allow Confirm to
						// replay for open / confirming / processing / completed
						// even though AllowsCustomerContinuation is open-only.
						if !allowsExternalConfirmReplay(locked.Status) {
							return checkoutSessionNotContinuableError(
								fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
							)
						}
					} else {
						if locked.Status == model.OpenCheckoutSessionStatus &&
							locked.ContractID == nil && locked.IsExpiredAt(now) {
							_, err = uc.transitionToExpired(ctx, tx, locked, now)
							if err != nil {
								return err
							}

							expired = true

							return nil
						}

						// First-time OTP verification requires an open, continuable
						// checkout. Processing/completed without prior verification
						// must never bypass the challenge.
						if !locked.Status.AllowsCustomerContinuation() {
							return checkoutSessionNotContinuableError(
								fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
							)
						}

						rejection, err = uc.consumeOTP(ctx, sessionQ, locked, input, now)
						if err != nil {
							return err
						}

						if rejection != nil {
							return nil
						}
					}

					profile, err := customerProfileQ.LockByID(ctx, *locked.CustomerID)
					if err != nil {
						return err
					}

					verified = verifiedExternalCheckout{
						sessionID:         locked.ID,
						identitySubjectID: profile.IdentitySubjectID,
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

// consumeOTP verifies the presented code against the stored verifier for the
// exact evaluation generation. Wrong codes increment the attempt counter;
// stale generation mismatches are rejected without burning an attempt.
//
// The returned rejection is a business outcome that must still commit (the
// failed attempt is persisted); err aborts and rolls the transaction back.
func (uc *UseCase) consumeOTP(
	ctx context.Context,
	sessionQ repo.CheckoutSessionTxQueryer,
	locked model.CheckoutSession,
	input ConfirmExternalInput,
	now time.Time,
) (rejection error, err error) {
	const op = "checkoutsessionuc.UseCase.consumeOTP"

	terms := uc.otpPolicy.Terms()

	if locked.OTPVerifier == nil || locked.OTPExpiresAt == nil {
		return nil, checkoutOTPNotRequestedError(
			fmt.Errorf("%s: no one-time code was issued", op),
		)
	}

	if locked.OTPEvaluationGeneration == nil ||
		*locked.OTPEvaluationGeneration != locked.EvaluationGeneration ||
		*locked.OTPEvaluationGeneration != input.EvaluationGeneration {
		// Offer terms changed after the code was issued. Do not burn an attempt
		// for a challenge that can no longer authorize this checkout.
		return nil, checkoutOfferReconfirmRequiredError(
			fmt.Errorf(
				"%s: otp challenge generation does not match evaluation generation %d",
				op,
				locked.EvaluationGeneration,
			),
		)
	}

	if locked.OTPAttemptCount >= terms.MaxVerificationAttempts {
		return nil, checkoutOTPLockedError(
			fmt.Errorf("%s: attempt budget is exhausted", op),
		)
	}

	if !now.Before(*locked.OTPExpiresAt) {
		return nil, checkoutOTPExpiredError(
			fmt.Errorf("%s: one-time code expired", op),
		)
	}

	matches, err := uc.otpHasher.Verify(
		checkoutotp.DomainSeparatedMaterial(
			input.Token.String(),
			input.EvaluationGeneration,
			input.OTP,
		),
		*locked.OTPVerifier,
	)
	if err != nil {
		return nil, cerr.InternalError(fmt.Errorf("%s: verify one-time code: %w", op, err))
	}

	if !matches {
		failed, failErr := sessionQ.RecordOTPFailure(ctx, locked.ID, now)
		if failErr != nil {
			return nil, failErr
		}

		if failed.OTPAttemptCount >= terms.MaxVerificationAttempts {
			return checkoutOTPLockedError(fmt.Errorf("%s: attempt budget is exhausted", op)), nil
		}

		return checkoutOTPInvalidError(fmt.Errorf("%s: one-time code does not match", op)), nil
	}

	_, err = sessionQ.MarkOTPVerified(ctx, locked.ID, now, now)
	if err != nil {
		return nil, err
	}

	return nil, nil
}
