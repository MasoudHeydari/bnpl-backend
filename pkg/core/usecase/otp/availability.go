package otp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Hosted OTP availability phases for countdown UX.
const (
	AvailabilityAwaitingEntry    = "awaiting_entry"
	AvailabilityConsumedWait     = "consumed_wait"
	AvailabilityLocked           = "locked"
	AvailabilityRequestAvailable = "request_available"
	AvailabilityIssuanceBlocked  = "issuance_blocked"
	AvailabilityNotApplicable    = "not_applicable"
)

// Availability returns a read-only privacy-safe OTP actionability snapshot.
func (uc *UseCase) Availability(
	ctx context.Context,
	input AvailabilityInput,
) (AvailabilityOutput, error) {
	const op = "otp.UseCase.Availability"

	if !input.Purpose.IsValid() {
		return AvailabilityOutput{}, fmt.Errorf("%s: unsupported purpose %q", op, input.Purpose)
	}

	if input.SubjectKey == "" {
		return AvailabilityOutput{}, fmt.Errorf("%s: subject key is required", op)
	}

	var (
		challenge model.OTPChallenge
		found     bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		otpQ := uc.challenges.Conn(conn)
		row, getErr := otpQ.GetByPurposeSubject(ctx, input.Purpose, input.SubjectKey)
		if getErr != nil {
			if cerr.IsCode(getErr, cerr.CodeResourceNotFound) {
				return nil
			}

			return getErr
		}

		challenge = row
		found = true

		return nil
	})
	if err != nil {
		return AvailabilityOutput{}, err
	}

	now := uc.clock.Now().UTC()
	terms := uc.policy.Terms()
	expectedContext := input.ContextVersion

	return AvailabilityOutput{
		Challenge:      challenge,
		Found:          found,
		OTPExpiresAt:   activeExpiryCopy(challenge, expectedContext, now),
		OTPNextIssueAt: nextIssueAtCopy(challenge),
		Availability: ProjectAvailability(
			challenge,
			expectedContext,
			now,
			terms.MaxVerificationAttempts,
			terms.MaxSendsPerSubject,
			input.Terminal,
			input.BusinessBlocked,
		),
	}, nil
}

// ProjectAvailability maps durable OTP timer fields and application hints into
// a customer-facing availability phase.
//
// Fixed priority:
//  1. terminal → not_applicable
//  2. locked (verification budget exhausted)
//  3. businessBlocked → issuance_blocked (even if a crypto-usable challenge exists)
//  4. awaiting_entry (only when bound to expectedContextVersion)
//  5. consumed_wait (original issuance window still active)
//  6. per-subject send budget exhausted → issuance_blocked
//  7. request_available
//
// SendCount is the durable lifetime counter enforced by Request; availability
// must not present request_available when a new Issue would be rejected for
// MaxSendsPerSubject. A still-usable code remains awaiting_entry even when the
// send budget is already exhausted.
func ProjectAvailability(
	challenge model.OTPChallenge,
	expectedContextVersion string,
	now time.Time,
	maxVerificationAttempts int,
	maxSendsPerSubject int,
	terminal bool,
	businessBlocked bool,
) string {
	if terminal {
		return AvailabilityNotApplicable
	}

	if maxVerificationAttempts > 0 && verificationLockedAt(challenge, maxVerificationAttempts) {
		return AvailabilityLocked
	}

	if businessBlocked {
		return AvailabilityIssuanceBlocked
	}

	if challengeUsableAt(challenge, expectedContextVersion, now) {
		return AvailabilityAwaitingEntry
	}

	if issuanceWindowActiveAt(challenge, now) {
		return AvailabilityConsumedWait
	}

	if maxSendsPerSubject > 0 && challenge.SendCount >= maxSendsPerSubject {
		return AvailabilityIssuanceBlocked
	}

	return AvailabilityRequestAvailable
}
