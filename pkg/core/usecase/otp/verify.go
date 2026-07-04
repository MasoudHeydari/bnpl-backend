package otp

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/otpcode"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// VerifyAndConsume verifies a still-unused OTP and atomically consumes it.
func (uc *UseCase) VerifyAndConsume(
	ctx context.Context,
	input VerifyInput,
) (VerifyOutput, error) {
	if err := validateVerifyInput(input); err != nil {
		return VerifyOutput{}, err
	}

	var (
		output    VerifyOutput
		rejection error
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			result, reject, verifyErr := uc.VerifyAndConsumeInTx(ctx, tx, input)
			if verifyErr != nil {
				return verifyErr
			}

			if reject != nil {
				rejection = reject
				output = result

				return nil
			}

			output = result

			return nil
		})
	})
	if err != nil {
		return VerifyOutput{}, err
	}

	if rejection != nil {
		// Wrong-code / lockout rejections are committed outcomes when RecordFailure
		// succeeded inside the transaction. Surface them as the return error.
		return output, rejection
	}

	return output, nil
}

// VerifyAndConsumeInTx verifies and consumes an OTP inside an existing
// transaction so callers can compose application locks with OTP consumption.
//
// A non-nil rejection is a business outcome that may still commit (failed
// attempt persisted). A non-nil error aborts the transaction.
func (uc *UseCase) VerifyAndConsumeInTx(
	ctx context.Context,
	tx repo.Tx,
	input VerifyInput,
) (output VerifyOutput, rejection error, err error) {
	const op = "otp.UseCase.VerifyAndConsumeInTx"

	if err := validateVerifyInput(input); err != nil {
		return VerifyOutput{}, nil, err
	}

	terms := uc.policy.Terms()
	otpQ := uc.challenges.Tx(tx)
	now := uc.clock.Now().UTC()

	locked, err := otpQ.EnsureAndLock(ctx, input.Purpose, input.SubjectKey, now)
	if err != nil {
		return VerifyOutput{}, nil, err
	}

	if locked.Verifier == nil || locked.ExpiresAt == nil {
		if locked.VerifiedAt != nil {
			return VerifyOutput{}, otpAlreadyConsumedError(
				fmt.Errorf("%s: one-time code was already consumed", op),
			), nil
		}

		return VerifyOutput{}, nil, otpNotRequestedError(
			fmt.Errorf("%s: no one-time code was issued", op),
		)
	}

	if locked.ContextVersion != input.ContextVersion {
		return VerifyOutput{}, nil, otpContextInvalidatedError(
			fmt.Errorf(
				"%s: otp challenge context version %q does not match %q",
				op,
				locked.ContextVersion,
				input.ContextVersion,
			),
		)
	}

	if locked.AttemptCount >= terms.MaxVerificationAttempts {
		return VerifyOutput{}, nil, otpLockedError(
			fmt.Errorf("%s: attempt budget is exhausted", op),
		)
	}

	if !now.Before(*locked.ExpiresAt) {
		return VerifyOutput{}, nil, otpExpiredError(
			fmt.Errorf("%s: one-time code expired", op),
		)
	}

	matches, err := uc.hasher.Verify(
		otpcode.DomainSeparatedMaterial(
			string(input.Purpose),
			input.Binding,
			input.ContextVersion,
			input.Code,
		),
		*locked.Verifier,
	)
	if err != nil {
		return VerifyOutput{}, nil, cerr.InternalError(fmt.Errorf("%s: verify one-time code: %w", op, err))
	}

	if !matches {
		failed, failErr := otpQ.RecordFailure(ctx, locked.ID, now)
		if failErr != nil {
			return VerifyOutput{}, nil, failErr
		}

		if failed.AttemptCount >= terms.MaxVerificationAttempts {
			return VerifyOutput{Challenge: failed}, otpLockedError(
				fmt.Errorf("%s: attempt budget is exhausted", op),
			), nil
		}

		return VerifyOutput{Challenge: failed}, otpInvalidError(
			fmt.Errorf("%s: one-time code does not match", op),
		), nil
	}

	if input.ConsumeAuthorizer != nil {
		if authErr := input.ConsumeAuthorizer.AuthorizeConsume(ctx, tx); authErr != nil {
			// Abort without MarkVerified so the challenge stays usable.
			return VerifyOutput{}, nil, authErr
		}
	}

	updated, err := otpQ.MarkVerified(
		ctx,
		locked.ID,
		input.ContextVersion,
		now,
		now,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return VerifyOutput{}, otpAlreadyConsumedError(
				fmt.Errorf("%s: one-time code was already consumed concurrently", op),
			), nil
		}

		return VerifyOutput{}, nil, err
	}

	return VerifyOutput{Challenge: updated}, nil, nil
}

func validateVerifyInput(input VerifyInput) error {
	const op = "otp.validateVerifyInput"

	if !input.Purpose.IsValid() {
		return otpInvalidError(fmt.Errorf("%s: unsupported purpose %q", op, input.Purpose))
	}

	if input.SubjectKey == "" {
		return otpInvalidError(fmt.Errorf("%s: subject key is required", op))
	}

	if input.ContextVersion == "" {
		return otpInvalidError(fmt.Errorf("%s: context version is required", op))
	}

	if input.Binding == "" {
		return otpInvalidError(fmt.Errorf("%s: binding is required", op))
	}

	if input.Code == "" {
		return otpRequiredError()
	}

	if !otpcode.IsWellFormed(input.Code) {
		return otpInvalidError(fmt.Errorf("%s: otp is not well-formed", op))
	}

	return nil
}
