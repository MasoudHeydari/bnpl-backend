package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// maxOTPNotificationRequestIDLength is the longest Notification request
// identifier the otp_notification_request_id column accepts.
const maxOTPNotificationRequestIDLength = 1024

// lockExternalSession locks the session addressed by its public token and
// requires it to be an external partner checkout.
//
// Internal checkouts are reported as missing: the hosted external routes carry
// no authentication, so they must never act on a session that is bound to a
// trusted customer by the privileged on-behalf API.
func (uc *UseCase) lockExternalSession(
	ctx context.Context,
	tx repo.Tx,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.lockExternalSession"

	sessionQ := uc.checkoutSession.Tx(tx)

	locked, err := sessionQ.LockByPublicToken(ctx, token)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CheckoutSession{}, checkoutSessionNotFoundError(err)
		}

		return model.CheckoutSession{}, err
	}

	if locked.Origin != model.ExternalPartnerCheckoutSessionOrigin {
		return model.CheckoutSession{}, checkoutSessionNotFoundError(
			fmt.Errorf("%s: checkout session origin is %q", op, locked.Origin),
		)
	}

	return locked, nil
}

// lockExternalContinuable locks an external session that still accepts
// customer continuation. An elapsed lifetime is persisted as expired and
// reported through expired=true; the caller must commit the transaction and
// then return checkoutSessionExpiredError, because returning the error from
// the transaction would roll the expiry back.
//
// Expiry is evaluated with clock.Now() after the row lock is held so waiting
// for FOR UPDATE cannot preserve a pre-deadline eligibility snapshot.
// When expiry wins, the returned session is the persisted expired row.
func (uc *UseCase) lockExternalContinuable(
	ctx context.Context,
	tx repo.Tx,
	token model.CheckoutSessionToken,
) (locked model.CheckoutSession, expired bool, err error) {
	const op = "checkoutsessionuc.UseCase.lockExternalContinuable"

	locked, err = uc.lockExternalSession(ctx, tx, token)
	if err != nil {
		return model.CheckoutSession{}, false, err
	}

	now := uc.clock.Now().UTC()

	if !locked.Status.AllowsCustomerContinuation() {
		return model.CheckoutSession{}, false, checkoutSessionNotContinuableError(
			fmt.Errorf("%s: checkout session status is %q", op, locked.Status),
		)
	}

	if locked.ContractID == nil && locked.IsExpiredAt(now) {
		persisted, transitionErr := uc.transitionToExpired(ctx, tx, locked, now)
		if transitionErr != nil {
			return model.CheckoutSession{}, false, transitionErr
		}

		return persisted, true, nil
	}

	return locked, false, nil
}

// allowsExternalConfirmReplay reports whether a previously OTP-verified
// external Confirm may resume into Confirm() for the given session status.
// Terminal abandon/deny states remain closed; open/confirming/processing/
// completed are valid replay targets after a lost response.
func allowsExternalConfirmReplay(status model.CheckoutSessionStatus) bool {
	switch status {
	case model.OpenCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.ProcessingCheckoutSessionStatus,
		model.CompletedCheckoutSessionStatus:
		return true

	default:
		return false
	}
}

// otpActiveAt reports whether the session needs no new one-time code at now
// for the current evaluation generation.
//
// An issued challenge reserves the slot whether or not MarkOTPNotificationAccepted has run:
// between IssueOTP commit and SMS acceptance another START must not overwrite
// the in-flight issuance. Verified phones for the same generation also count.
func otpActiveAt(session model.CheckoutSession, now time.Time) bool {
	if session.OTPEvaluationGeneration == nil ||
		*session.OTPEvaluationGeneration != session.EvaluationGeneration {
		return false
	}

	if session.OTPVerifiedAt != nil {
		return true
	}

	return session.OTPIssueID != nil &&
		!session.OTPIssueID.IsZero() &&
		session.OTPVerifier != nil &&
		session.OTPExpiresAt != nil &&
		now.Before(*session.OTPExpiresAt)
}

// activeOTPExpiry returns the expiry of the code a customer can still enter, or
// nil when none is usable.
func activeOTPExpiry(session model.CheckoutSession, now time.Time) *time.Time {
	if session.OTPVerifiedAt != nil || session.OTPVerifier == nil || session.OTPExpiresAt == nil {
		return nil
	}

	if !now.Before(*session.OTPExpiresAt) {
		return nil
	}

	expiresAt := *session.OTPExpiresAt

	return &expiresAt
}

// normalizeOTPNotificationRequestID bounds a Notification request_id to the
// column limit. Empty values are rejected by MarkOTPNotificationAccepted; this
// helper is retained for defensive adapters that must clip unusually long ids.
func normalizeOTPNotificationRequestID(requestID string) string {
	if len(requestID) > maxOTPNotificationRequestIDLength {
		return requestID[:maxOTPNotificationRequestIDLength]
	}

	return requestID
}
