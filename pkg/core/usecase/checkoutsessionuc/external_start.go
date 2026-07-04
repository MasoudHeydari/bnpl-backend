package checkoutsessionuc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/checkoutotp"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

const (
	// otpNotificationCodeParam is the template placeholder carrying the OTP.
	otpNotificationCodeParam = "code"

	// otpNotificationMerchantNameParam is the template placeholder carrying the
	// trusted BNPL merchant display name (never a partner-supplied label).
	otpNotificationMerchantNameParam = "merchant_name"

	// otpNotificationIdempotencyPrefix namespaces the Notification idempotency
	// key of one OTP issuance.
	otpNotificationIdempotencyPrefix = "checkout-otp:"
)

// externalOTPRequestState is what StartExternal learns before submitting OTP.
type externalOTPRequestState struct {
	session model.CheckoutSession
	phone   string
	offer   Offer
}

// StartExternal requests OTP delivery for a prepared External Hosted checkout.
//
// The financing offer must already have been prepared during Partner Create and
// shown by Hosted GET. This method does not call Identity, bind a customer,
// create/refresh a Decision, or invent new offer terms.
//
// Flow:
//  1. Validate token + evaluation_generation against the persisted session/offer.
//  2. Load trusted merchant display name (DB read; no network).
//  3. Tx: apply OTP rate limits + IssueOTP (no network).
//  4. Outside Tx: Notification SubmitNotification.
//  5. Tx: MarkOTPNotificationAccepted (CAS on otp_issue_id).
//
// A stale evaluation_generation consumes no OTP budgets and submits nothing.
// An active/in-flight challenge for the same generation is idempotent.
func (uc *UseCase) StartExternal(
	ctx context.Context,
	input StartExternalInput,
) (StartExternalOutput, error) {
	const op = "checkoutsessionuc.UseCase.StartExternal"

	if err := input.validate(); err != nil {
		return StartExternalOutput{}, err
	}

	state, err := uc.beginRequestOTP(ctx, input)
	if err != nil {
		return StartExternalOutput{}, err
	}

	now := uc.clock.Now().UTC()
	output := StartExternalOutput{
		Session:              state.session,
		EvaluationGeneration: state.session.EvaluationGeneration,
		OTPExpiresAt:         activeOTPExpiry(state.session, now),
	}

	// Load the trusted merchant display name before consuming OTP budgets.
	// A failure here must not leave an issued challenge or incremented
	// cross-checkout counters with no Notification attempt.
	merchantName, err := uc.merchantDisplayName(ctx, state.session.MerchantProfileID)
	if err != nil {
		return StartExternalOutput{}, err
	}

	issued, err := uc.issueOTP(ctx, input.Token, input.EvaluationGeneration)
	if err != nil {
		return StartExternalOutput{}, err
	}

	output.Session = issued.session
	output.EvaluationGeneration = issued.session.EvaluationGeneration
	output.OTPExpiresAt = activeOTPExpiry(issued.session, uc.clock.Now().UTC())

	if !issued.issued {
		return output, nil
	}

	requestID, err := uc.submitOTPNotification(ctx, input.Token, state.phone, merchantName, issued)
	if err != nil {
		// Definite Notification rejection clears this exact issuance
		// (otp_issue_id CAS) so a later START may issue a new OTP.
		//
		// Ambiguous failures (timeout / unavailable / ALREADY_EXISTS) retain
		// the issuance until it expires. BNPL does not resubmit the same OTP:
		// the plaintext code is intentionally not persisted after this request.
		if notification.IsRejected(err) {
			if clearErr := uc.clearOTPIssue(ctx, issued.session.ID, issued.issueID); clearErr != nil {
				return StartExternalOutput{}, errors.Join(err, clearErr)
			}
		}

		return StartExternalOutput{}, err
	}

	accepted, err := uc.markOTPNotificationAccepted(ctx, issued.session.ID, issued.issueID, requestID)
	if err != nil {
		return StartExternalOutput{}, err
	}

	output.Session = accepted
	output.OTPSent = true
	output.OTPExpiresAt = activeOTPExpiry(accepted, uc.clock.Now().UTC())

	return output, nil
}

// beginRequestOTP validates that the checkout is ready for OTP issuance for the
// exact evaluation_generation the customer saw. It never calls Identity or
// Decision creation.
func (uc *UseCase) beginRequestOTP(
	ctx context.Context,
	input StartExternalInput,
) (externalOTPRequestState, error) {
	const op = "checkoutsessionuc.UseCase.beginRequestOTP"

	var (
		state   externalOTPRequestState
		expired bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					locked, lockedExpired, err := uc.lockExternalContinuable(ctx, tx, input.Token)
					if err != nil {
						return err
					}

					if lockedExpired {
						expired = true

						return nil
					}

					if locked.EvaluationGeneration != input.EvaluationGeneration {
						return checkoutOfferReconfirmRequiredError(
							fmt.Errorf(
								"%s: displayed generation %d does not match current %d",
								op,
								input.EvaluationGeneration,
								locked.EvaluationGeneration,
							),
						)
					}

					if locked.Status != model.OpenCheckoutSessionStatus {
						return checkoutSessionNotContinuableError(
							fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
						)
					}

					if locked.ContractID != nil {
						return checkoutSessionNotContinuableError(
							fmt.Errorf("%s: checkout already has a contract", op),
						)
					}

					if locked.CustomerID == nil {
						return checkoutOfferNotReadyError(
							fmt.Errorf("%s: customer identity is not resolved", op),
						)
					}

					if locked.DecisionID == nil || locked.DecisionID.IsZero() {
						return checkoutOfferNotReadyError(
							fmt.Errorf("%s: financing decision is not prepared", op),
						)
					}

					phone, err := uc.decryptCustomerPhone(locked)
					if err != nil {
						return err
					}

					state.session = locked
					state.phone = phone

					return nil
				},
			)
		},
	)
	if err != nil {
		return externalOTPRequestState{}, err
	}

	if expired {
		return externalOTPRequestState{}, checkoutSessionExpiredError()
	}

	offer, err := uc.projectPersistedOffer(ctx, state.session)
	if err != nil {
		return externalOTPRequestState{}, err
	}

	if offer.Outcome != ApprovedOfferOutcome {
		return externalOTPRequestState{}, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: offer outcome is %q", op, offer.Outcome),
		)
	}

	now := uc.clock.Now().UTC()
	if offer.OfferExpiresAt != nil && !offer.OfferExpiresAt.After(now) {
		return externalOTPRequestState{}, checkoutSessionExpiredError()
	}

	state.offer = offer

	return state, nil
}

// bindExternalCustomer attaches the customer resolved by the Identity Service
// to the session, creating the BNPL profile when this is its first checkout.
//
// This runs during Partner Create offer preparation. Binding records Identity
// resolution (phone → customer), not OTP possession proof or purchase
// authorization. A session already bound to a different customer fails closed.
func (uc *UseCase) bindExternalCustomer(
	ctx context.Context,
	token model.CheckoutSessionToken,
	identitySubjectID model.IdentitySubjectID,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.bindExternalCustomer"

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
					customerProfileQ := uc.customerProfile.Tx(tx)

					locked, lockedExpired, err := uc.lockExternalContinuable(ctx, tx, token)
					if err != nil {
						return err
					}

					if lockedExpired {
						expired = true

						return nil
					}

					now := uc.clock.Now().UTC()

					profile, _, err := customerProfileQ.EnsureExists(
						ctx,
						identitySubjectID,
						model.IRJurisdiction,
						model.ActiveCustomerStatus,
						now,
						now,
					)
					if err != nil {
						return err
					}

					if locked.CustomerID != nil && *locked.CustomerID != profile.ID {
						return checkoutCustomerIdentityConflictError(
							fmt.Errorf("%s: session belongs to another customer", op),
						)
					}

					bound, err := sessionQ.BindCustomer(ctx, locked.ID, profile.ID, now)
					if err != nil {
						return err
					}

					session = bound

					return nil
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

	return session, nil
}

// issuedOTP is the result of the OTP issuing transaction.
type issuedOTP struct {
	session   model.CheckoutSession
	issued    bool
	issueID   model.OTPIssueID
	code      string
	expiresAt time.Time
}

// issueOTP generates a code and stores only its keyed verifier, bound to the
// session's current evaluation generation under a fresh otp_issue_id.
//
// expectedGeneration must match the locked session before any rate-limit
// consumption. Generation and hashing are local CPU work inside the
// transaction; there is no network call.
func (uc *UseCase) issueOTP(
	ctx context.Context,
	token model.CheckoutSessionToken,
	expectedGeneration int,
) (issuedOTP, error) {
	const op = "checkoutsessionuc.UseCase.issueOTP"

	terms := uc.otpPolicy.Terms()

	var (
		result    issuedOTP
		expired   bool
		rejection error
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)

					locked, lockedExpired, err := uc.lockExternalContinuable(ctx, tx, token)
					if err != nil {
						return err
					}

					if lockedExpired {
						expired = true

						return nil
					}

					now := uc.clock.Now().UTC()

					// Stale generation must not consume phone/merchant budgets.
					if locked.EvaluationGeneration != expectedGeneration {
						return checkoutOfferReconfirmRequiredError(
							fmt.Errorf(
								"%s: displayed generation %d does not match current %d",
								op,
								expectedGeneration,
								locked.EvaluationGeneration,
							),
						)
					}

					if otpActiveAt(locked, now) {
						result.session = locked

						return nil
					}

					if locked.OTPSendCount >= terms.MaxSendsPerCheckout {
						rejection = checkoutOTPSendLimitError(
							fmt.Errorf("%s: otp send budget exhausted", op),
						)

						return nil
					}

					if locked.OTPNotificationAcceptedAt != nil && terms.SendCooldown > 0 {
						cooldownUntil := locked.OTPNotificationAcceptedAt.Add(terms.SendCooldown)
						if now.Before(cooldownUntil) {
							rejection = checkoutOTPSendCooldownError(
								fmt.Errorf("%s: otp send cooldown active until %s", op, cooldownUntil.UTC()),
							)

							return nil
						}
					}

					phone, err := uc.decryptCustomerPhone(locked)
					if err != nil {
						return err
					}

					phoneFingerprint, err := uc.otpHasher.Hash(checkoutotp.PhoneRateLimitMaterial(phone))
					if err != nil {
						return fmt.Errorf("%s: hash phone rate-limit key: %w", op, err)
					}

					// Cross-checkout rate-limit rejection must return an error
					// from this transaction callback so PostgreSQL rolls back.
					if err := sessionQ.ConsumeOTPRateLimit(
						ctx,
						model.CheckoutOTPRateLimitScopePhone,
						phoneFingerprint.String(),
						terms.PhoneSendWindow,
						terms.MaxSendsPerPhone,
						now,
					); err != nil {
						if cerr.IsCode(err, cerr.CodeTooManyRequests) {
							return checkoutOTPRateLimitedError(err)
						}

						return err
					}

					if err := sessionQ.ConsumeOTPRateLimit(
						ctx,
						model.CheckoutOTPRateLimitScopeMerchant,
						locked.MerchantProfileID.String(),
						terms.MerchantSendWindow,
						terms.MaxSendsPerMerchant,
						now,
					); err != nil {
						if cerr.IsCode(err, cerr.CodeTooManyRequests) {
							return checkoutOTPRateLimitedError(err)
						}

						return err
					}

					issueID, err := model.GenerateOTPIssueID(uc.random)
					if err != nil {
						return fmt.Errorf("%s: generate otp issue id: %w", op, err)
					}

					code, err := checkoutotp.GenerateSixDigitOTP(uc.random)
					if err != nil {
						return fmt.Errorf("%s: generate otp: %w", op, err)
					}

					material := checkoutotp.DomainSeparatedMaterial(
						token.String(),
						locked.EvaluationGeneration,
						code,
					)

					verifier, err := uc.otpHasher.Hash(material)
					if err != nil {
						return fmt.Errorf("%s: hash otp: %w", op, err)
					}

					expiresAt := now.Add(terms.TTL)

					updated, err := sessionQ.IssueOTP(
						ctx,
						locked.ID,
						issueID,
						verifier.String(),
						locked.EvaluationGeneration,
						expiresAt,
						now,
					)
					if err != nil {
						return err
					}

					result = issuedOTP{
						session:   updated,
						issued:    true,
						issueID:   issueID,
						code:      code,
						expiresAt: expiresAt,
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return issuedOTP{}, err
	}

	if expired {
		return issuedOTP{}, checkoutSessionExpiredError()
	}

	if rejection != nil {
		return issuedOTP{}, rejection
	}

	return result, nil
}

// merchantDisplayName loads the trusted BNPL merchant display name for OTP
// notification templating. Partner request labels are never used.
func (uc *UseCase) merchantDisplayName(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
) (string, error) {
	const op = "checkoutsessionuc.UseCase.merchantDisplayName"

	var name string

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantQ := uc.merchantProfile.Conn(conn)
		merchant, getErr := merchantQ.GetByID(ctx, merchantProfileID)
		if getErr != nil {
			return getErr
		}

		name = merchant.DisplayName

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("%s: load merchant profile: %w", op, err)
	}

	if name == "" {
		return "", cerr.InternalError(fmt.Errorf("%s: merchant display name is empty", op))
	}

	return name, nil
}

// submitOTPNotification submits the OTP through the Notification Service. It
// must run outside any transaction. The idempotency key is stable per
// otp_issue_id so an in-process transport retry cannot create a second logical
// submission. expires_at is the exact BNPL OTP expiry for this issuance.
// After this request ends BNPL cannot resubmit the same issuance: the plaintext
// OTP is not persisted. A later START either reuses an unexpired in-flight
// challenge (idempotent, no new submission) or, after definite rejection /
// expiry, issues a new challenge with a new key.
func (uc *UseCase) submitOTPNotification(
	ctx context.Context,
	token model.CheckoutSessionToken,
	phone string,
	merchantName string,
	issued issuedOTP,
) (string, error) {
	const op = "checkoutsessionuc.UseCase.submitOTPNotification"

	out, err := uc.notificationSubmitter.Submit(ctx, notification.SubmitInput{
		TemplateID:     uc.otpTemplateID,
		Recipient:      phone,
		IdempotencyKey: otpNotificationIdempotencyPrefix + token.String() + ":" + issued.issueID.String(),
		CorrelationID:  trace.CorrelationID(ctx),
		ExpiresAt:      issued.expiresAt,
		Variables: map[string]string{
			otpNotificationCodeParam:         issued.code,
			otpNotificationMerchantNameParam: merchantName,
		},
	})
	if err != nil {
		return "", fmt.Errorf("%s: submit notification: %w", op, err)
	}

	if out.RequestID == "" {
		return "", cerr.InternalError(fmt.Errorf("%s: notification returned an empty request id", op))
	}

	return out.RequestID, nil
}

func (uc *UseCase) markOTPNotificationAccepted(
	ctx context.Context,
	id model.CheckoutSessionID,
	issueID model.OTPIssueID,
	requestID string,
) (model.CheckoutSession, error) {
	var session model.CheckoutSession

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)
			now := uc.clock.Now().UTC()

			updated, err := sessionQ.MarkOTPNotificationAccepted(ctx, id, issueID, now, requestID, now)
			if err != nil {
				return err
			}

			session = updated

			return nil
		})
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	return session, nil
}

func (uc *UseCase) clearOTPIssue(
	ctx context.Context,
	id model.CheckoutSessionID,
	issueID model.OTPIssueID,
) error {
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)
			_, err := sessionQ.ClearOTPIssue(ctx, id, issueID, uc.clock.Now().UTC())

			return err
		})
	})
}
