package otp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// submitNotification submits the OTP through the Notification Service. It must
// run outside any transaction. The idempotency key is stable per issueID so an
// in-process transport retry cannot create a second logical submission.
func (uc *UseCase) submitNotification(
	ctx context.Context,
	auth AuthorizedIssuance,
	issueID model.OTPIssueID,
	code string,
	expiresAt time.Time,
) (string, error) {
	const op = "otp.UseCase.submitNotification"

	out, err := uc.notification.Submit(ctx, notification.SubmitInput{
		TemplateID:     auth.TemplateID,
		Recipient:      auth.Recipient,
		IdempotencyKey: auth.IdempotencyKeyPrefix + issueID.String(),
		CorrelationID:  auth.CorrelationID,
		ExpiresAt:      expiresAt,
		Variables:      withCodeVariable(auth.Variables, code),
	})
	if err != nil {
		return "", fmt.Errorf("%s: submit notification: %w", op, err)
	}

	if out.RequestID == "" {
		return "", cerr.InternalError(fmt.Errorf("%s: notification returned an empty request id", op))
	}

	return out.RequestID, nil
}

func withCodeVariable(vars map[string]string, code string) map[string]string {
	out := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		out[k] = v
	}

	out["code"] = code

	return out
}

func (uc *UseCase) markNotificationAccepted(
	ctx context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	requestID string,
) (model.OTPChallenge, error) {
	var challenge model.OTPChallenge

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			otpQ := uc.challenges.Tx(tx)
			now := uc.clock.Now().UTC()

			updated, err := otpQ.MarkNotificationAccepted(
				ctx,
				id,
				issueID,
				now,
				requestID,
				now,
			)
			if err != nil {
				return err
			}

			challenge = updated

			return nil
		})
	})
	if err != nil {
		return model.OTPChallenge{}, err
	}

	return challenge, nil
}

func (uc *UseCase) clearIssue(
	ctx context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
) error {
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			otpQ := uc.challenges.Tx(tx)
			_, err := otpQ.ClearIssue(ctx, id, issueID, uc.clock.Now().UTC())

			return err
		})
	})
}

func (uc *UseCase) handleNotificationFailure(
	ctx context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	err error,
) error {
	if notification.IsRejected(err) {
		if clearErr := uc.clearIssue(ctx, id, issueID); clearErr != nil {
			return errors.Join(err, clearErr)
		}
	}

	return err
}
