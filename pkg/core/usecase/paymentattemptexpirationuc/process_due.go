package paymentattemptexpirationuc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ExpireDueResult summarizes one expiration cycle for observability.
type ExpireDueResult struct {
	Claimed             int
	Expired             int
	SkippedChangedState int
	SkippedActiveLease  int
	SkippedNotEligible  int
	Failures            int
	Duration            time.Duration
}

// ProcessDue claims eligible installment repayment attempts and expires them
// locally. It never calls Financial Core. Each candidate is processed in its
// own short transaction so concurrent workers and late FC writers remain safe.
func (uc *UseCase) ProcessDue(ctx context.Context) error {
	const op = "paymentattemptexpirationuc.UseCase.ProcessDue"

	started := uc.clock.Now().UTC()
	result := ExpireDueResult{}
	remaining := uc.batchSize

	var batchErr error
	for remaining > 0 {
		processed, err := uc.expireNextCandidate(ctx, started, &result)
		if err != nil {
			result.Failures++
			batchErr = errors.Join(batchErr, err)
			break
		}

		if !processed {
			break
		}

		remaining--
	}

	result.Duration = uc.clock.Now().UTC().Sub(started)
	uc.logger.With(
		"claimed", result.Claimed,
		"expired", result.Expired,
		"skipped_changed_state", result.SkippedChangedState,
		"skipped_active_lease", result.SkippedActiveLease,
		"skipped_not_eligible", result.SkippedNotEligible,
		"failures", result.Failures,
		"duration_ms", result.Duration.Milliseconds(),
	).Info("payment attempt session creation expiration cycle completed")

	if batchErr != nil {
		return fmt.Errorf("%s: %w", op, batchErr)
	}

	return nil
}

func (uc *UseCase) expireNextCandidate(
	ctx context.Context,
	now time.Time,
	result *ExpireDueResult,
) (bool, error) {
	const op = "paymentattemptexpirationuc.UseCase.expireNextCandidate"

	var processed bool
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)

			candidates, claimErr := paymentAttemptQ.ClaimCandidatesForSessionCreationExpiration(
				ctx,
				now,
				uc.safetyAfterLease,
				1,
			)
			if claimErr != nil {
				return fmt.Errorf("%s: claim candidates: %w", op, claimErr)
			}

			if len(candidates) == 0 {
				return nil
			}

			result.Claimed++
			processed = true
			candidate := candidates[0]

			locked, lockErr := paymentAttemptQ.LockByID(ctx, candidate.ID)
			if lockErr != nil {
				if cerr.IsCode(lockErr, cerr.CodeResourceNotFound) {
					result.SkippedChangedState++
					return nil
				}

				return fmt.Errorf("%s: lock payment attempt %s: %w", op, candidate.ID, lockErr)
			}

			if locked.State == model.ExpiredPaymentAttemptState {
				result.SkippedChangedState++
				return nil
			}

			if locked.FinancialSessionCreateLeaseActive(now) {
				result.SkippedActiveLease++
				return nil
			}

			if !locked.EligibleForSessionCreationExpiration(now, uc.safetyAfterLease) {
				result.SkippedNotEligible++
				return nil
			}

			outboxQ := uc.outbox.Tx(tx)
			expectedState := locked.State

			// A crashed third CreateCheckoutPayment finalization must become
			// locally canceled after the lease safety window — never a fourth FC call.
			if locked.FinancialSessionCreateAttemptCount >= model.MaxFinancialSessionCreateAttempts {
				if err := locked.State.ValidateTransitionTo(model.CanceledPaymentAttemptState); err != nil {
					result.SkippedChangedState++
					return nil
				}

				canceled, cancelErr := paymentAttemptQ.CancelSessionCreationRetryExhausted(
					ctx,
					locked.ID,
					expectedState,
					now,
					now,
				)
				if cancelErr != nil {
					if cerr.IsCode(cancelErr, cerr.CodeConflict) {
						result.SkippedChangedState++
						return nil
					}

					return fmt.Errorf("%s: cancel exhausted session creation %s: %w", op, locked.ID, cancelErr)
				}

				event := uc.newRetryExhaustedCancelAuditEvent(locked, canceled, now)
				if auditErr := uc.insertAuditOutbox(ctx, outboxQ, event, now); auditErr != nil {
					return fmt.Errorf("%s: insert cancellation audit for %s: %w", op, locked.ID, auditErr)
				}

				result.Expired++
				return nil
			}

			if err := locked.State.ValidateTransitionTo(model.ExpiredPaymentAttemptState); err != nil {
				result.SkippedChangedState++
				return nil
			}

			expired, markErr := paymentAttemptQ.MarkExpired(
				ctx,
				locked.ID,
				expectedState,
				now,
				now,
			)
			if markErr != nil {
				if cerr.IsCode(markErr, cerr.CodeConflict) {
					result.SkippedChangedState++
					return nil
				}

				return fmt.Errorf("%s: mark expired %s: %w", op, locked.ID, markErr)
			}

			event := uc.newExpiredAuditEvent(locked, expired, now)
			if auditErr := uc.insertAuditOutbox(ctx, outboxQ, event, now); auditErr != nil {
				return fmt.Errorf("%s: insert expiration audit for %s: %w", op, locked.ID, auditErr)
			}

			result.Expired++
			return nil
		})
	})
	if err != nil {
		return processed, err
	}

	return processed, nil
}
