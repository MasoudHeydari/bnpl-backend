package checkoutsessionuc

import (
	"context"
	"strconv"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

// Hosted OTP availability phases for countdown UX. Aliases of otp package
// constants kept for HTTP-layer compatibility.
const (
	OTPAvailabilityAwaitingEntry    = otp.AvailabilityAwaitingEntry
	OTPAvailabilityConsumedWait     = otp.AvailabilityConsumedWait
	OTPAvailabilityLocked           = otp.AvailabilityLocked
	OTPAvailabilityRequestAvailable = otp.AvailabilityRequestAvailable
	OTPAvailabilityIssuanceBlocked  = otp.AvailabilityIssuanceBlocked
	OTPAvailabilityNotApplicable    = otp.AvailabilityNotApplicable
)

// projectActionableOTPAvailability includes durable business blockers that would
// make POST /otp reject issuance. It is read-only and never calls external
// services. Cross-checkout rate limits remain authoritative only at request time.
func (uc *UseCase) projectActionableOTPAvailability(
	ctx context.Context,
	session model.CheckoutSession,
	status model.CheckoutSessionStatus,
	now time.Time,
) (string, error) {
	blocked, err := uc.evaluateOTPIssuanceBlockersOutsideTx(ctx, session, status, now)
	if err != nil {
		return "", err
	}

	out, err := uc.otp.Availability(ctx, otp.AvailabilityInput{
		Purpose:         model.OTPPurposeHostedCheckoutConfirm,
		SubjectKey:      session.ID.String(),
		ContextVersion:  strconv.Itoa(session.EvaluationGeneration),
		Terminal:        isTerminalCheckoutStatus(status),
		BusinessBlocked: blocked,
	})
	if err != nil {
		return "", err
	}

	return out.Availability, nil
}

// otpIssuanceBlockedByConfirmation maps confirmation_status into a durable
// issuance blocker for status GET without reloading Contract/PaymentAttempt.
func otpIssuanceBlockedByConfirmation(confirmationStatus string, session model.CheckoutSession) bool {
	switch confirmationStatus {
	case ConfirmationStatusNeedsAttention,
		ConfirmationStatusCompleted,
		ConfirmationStatusCanceled,
		ConfirmationStatusExpired,
		ConfirmationStatusUnavailable,
		ConfirmationStatusReconfirmRequired:
		return true

	case ConfirmationStatusNotStarted:
		return session.DecisionID == nil || session.DecisionID.IsZero() ||
			session.CustomerID == nil || session.CustomerID.IsZero()

	default:
		return false
	}
}

func isTerminalCheckoutStatus(status model.CheckoutSessionStatus) bool {
	switch status {
	case model.CompletedCheckoutSessionStatus,
		model.CanceledCheckoutSessionStatus,
		model.ExpiredCheckoutSessionStatus,
		model.UnavailableCheckoutSessionStatus:
		return true

	default:
		return false
	}
}
