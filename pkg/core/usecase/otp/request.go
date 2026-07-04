package otp

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/otpcode"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// issuedChallenge is the result of the OTP issuing transaction.
type issuedChallenge struct {
	challenge model.OTPChallenge
	auth      AuthorizedIssuance
	issued    bool
	issueID   model.OTPIssueID
	code      string
	expiresAt time.Time
}

// Request issues (or replays) an OTP challenge for the authorized subject.
//
// Flow:
//  1. Tx: authorize + EnsureAndLock + rate limits + Issue (no network).
//  2. Outside Tx: Notification Submit — BEFORE any availability projection.
//  3. Tx: MarkNotificationAccepted (CAS on issue_id).
//  4. Project availability; projection failure after notify must not fail the
//     request (return OTPSent with timers).
//
// Delivery (steps 2–3) uses a detached context with notificationDeliveryTimeout
// so an HTTP client cancel after Issue commits cannot strand an issued challenge
// mid-delivery. Crash window: plaintext is never stored; an interrupted
// delivery occupies the issuance window until expiry; a retry replays the unused
// challenge without a second SMS while the challenge remains usable.
func (uc *UseCase) Request(ctx context.Context, input RequestInput) (RequestOutput, error) {
	if err := validateRequestInput(input); err != nil {
		return RequestOutput{}, err
	}

	issued, err := uc.issue(ctx, input)
	if err != nil {
		return RequestOutput{}, err
	}

	if !issued.issued {
		now := uc.clock.Now().UTC()
		terms := uc.policy.Terms()

		return RequestOutput{
			Challenge:      issued.challenge,
			OTPExpiresAt:   activeExpiryCopy(issued.challenge, input.ContextVersion, now),
			OTPNextIssueAt: nextIssueAtCopy(issued.challenge),
			Availability: ProjectAvailability(
				issued.challenge,
				input.ContextVersion,
				now,
				terms.MaxVerificationAttempts,
				terms.MaxSendsPerSubject,
				issued.auth.Terminal,
				issued.auth.BusinessBlocked,
			),
		}, nil
	}

	deliveryCtx, cancelDelivery := notificationDeliveryContext(ctx, uc.notificationTimeout)
	defer cancelDelivery()

	// CRITICAL: submit Notification before any availability projection work.
	requestID, err := uc.submitNotification(
		deliveryCtx,
		issued.auth,
		issued.issueID,
		issued.code,
		issued.expiresAt,
	)
	if err != nil {
		return RequestOutput{}, uc.handleNotificationFailure(
			deliveryCtx,
			issued.challenge.ID,
			issued.issueID,
			err,
		)
	}

	accepted, err := uc.markNotificationAccepted(
		deliveryCtx,
		issued.challenge.ID,
		issued.issueID,
		requestID,
	)
	if err != nil {
		return RequestOutput{}, err
	}

	acceptedNow := uc.clock.Now().UTC()
	terms := uc.policy.Terms()
	output := RequestOutput{
		Challenge:      accepted,
		OTPSent:        true,
		OTPExpiresAt:   activeExpiryCopy(accepted, input.ContextVersion, acceptedNow),
		OTPNextIssueAt: nextIssueAtCopy(accepted),
		Availability: ProjectAvailability(
			accepted,
			input.ContextVersion,
			acceptedNow,
			terms.MaxVerificationAttempts,
			terms.MaxSendsPerSubject,
			issued.auth.Terminal,
			issued.auth.BusinessBlocked,
		),
	}

	// Projection failure after notify must not fail the request: the SMS was
	// already accepted. Pure ProjectAvailability cannot fail; test hooks may.
	if uc.afterNotifyProject != nil {
		if projectErr := uc.afterNotifyProject(ctx); projectErr != nil {
			return output, nil
		}
	}

	return output, nil
}

func validateRequestInput(input RequestInput) error {
	const op = "otp.validateRequestInput"

	if !input.Purpose.IsValid() {
		return otpIssuanceNotAllowedError(fmt.Errorf("%s: unsupported purpose %q", op, input.Purpose))
	}

	if input.SubjectKey == "" {
		return otpIssuanceNotAllowedError(fmt.Errorf("%s: subject key is required", op))
	}

	if input.ContextVersion == "" {
		return otpIssuanceNotAllowedError(fmt.Errorf("%s: context version is required", op))
	}

	if input.Authorizer == nil {
		return fmt.Errorf("%s: issuance authorizer is required", op)
	}

	return nil
}

func (uc *UseCase) issue(ctx context.Context, input RequestInput) (issuedChallenge, error) {
	const op = "otp.UseCase.issue"

	terms := uc.policy.Terms()

	var (
		result    issuedChallenge
		rejection error
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			auth, err := input.Authorizer.AuthorizeIssuance(ctx, tx)
			if err != nil {
				return err
			}

			if err := validateAuthorizedIssuance(auth, input); err != nil {
				return err
			}

			otpQ := uc.challenges.Tx(tx)
			now := uc.clock.Now().UTC()

			locked, err := otpQ.EnsureAndLock(ctx, input.Purpose, input.SubjectKey, now)
			if err != nil {
				return err
			}

			result.auth = auth

			if challengeUsableAt(locked, input.ContextVersion, now) {
				result.challenge = locked

				return nil
			}

			if issuanceWindowActiveAt(locked, now) {
				rejection = otpIssuanceWindowActiveError(
					fmt.Errorf("%s: otp issuance window active until %s", op, locked.NextIssueAt.UTC()),
					locked,
					now,
				)

				return nil
			}

			if locked.SendCount >= terms.MaxSendsPerSubject {
				rejection = otpSendLimitError(fmt.Errorf("%s: otp send budget exhausted", op))

				return nil
			}

			if locked.NotificationAcceptedAt != nil && terms.SendCooldown > 0 {
				cooldownUntil := locked.NotificationAcceptedAt.Add(terms.SendCooldown)
				if now.Before(cooldownUntil) {
					rejection = otpSendCooldownError(
						fmt.Errorf("%s: otp send cooldown active until %s", op, cooldownUntil.UTC()),
					)

					return nil
				}
			}

			// Stable lock order for rate-limit rows regardless of authorizer
			// list order: (Scope, ScopeKey) lexicographic.
			rateLimits := append([]RateLimitBudget(nil), auth.RateLimits...)
			sortRateLimitBudgets(rateLimits)

			for _, budget := range rateLimits {
				if err := otpQ.ConsumeRateLimit(
					ctx,
					budget.Scope,
					budget.ScopeKey,
					budget.Window,
					budget.MaxSends,
					now,
				); err != nil {
					if cerr.IsCode(err, cerr.CodeTooManyRequests) {
						return otpRateLimitedError(err)
					}

					return err
				}
			}

			if hook := uc.beforeIssueCommit; hook != nil {
				if err := hook(ctx); err != nil {
					return err
				}

				// Re-authorize after the test/operator barrier so a concurrent
				// terminal application transition wins before Issue persists.
				auth, err = input.Authorizer.AuthorizeIssuance(ctx, tx)
				if err != nil {
					return err
				}

				if err := validateAuthorizedIssuance(auth, input); err != nil {
					return err
				}

				result.auth = auth
			}

			issueID, err := model.GenerateOTPIssueID(uc.random)
			if err != nil {
				return fmt.Errorf("%s: generate otp issue id: %w", op, err)
			}

			code, err := otpcode.GenerateSixDigitOTP(uc.random)
			if err != nil {
				return fmt.Errorf("%s: generate otp: %w", op, err)
			}

			material := otpcode.DomainSeparatedMaterial(
				string(input.Purpose),
				auth.Binding,
				input.ContextVersion,
				code,
			)

			verifier, err := uc.hasher.Hash(material)
			if err != nil {
				return fmt.Errorf("%s: hash otp: %w", op, err)
			}

			expiresAt := now.Add(terms.TTL)

			updated, err := otpQ.Issue(
				ctx,
				locked.ID,
				issueID,
				verifier.String(),
				input.ContextVersion,
				expiresAt,
				now,
			)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					reloaded, reloadErr := otpQ.EnsureAndLock(ctx, input.Purpose, input.SubjectKey, now)
					if reloadErr != nil {
						return reloadErr
					}

					if challengeUsableAt(reloaded, input.ContextVersion, now) {
						result.challenge = reloaded

						return nil
					}

					if issuanceWindowActiveAt(reloaded, now) {
						rejection = otpIssuanceWindowActiveError(
							fmt.Errorf("%s: concurrent otp issuance already occupied the window", op),
							reloaded,
							now,
						)

						return nil
					}

					return otpIssuanceNotAllowedError(
						fmt.Errorf("%s: concurrent otp issuance race left no usable challenge", op),
					)
				}

				return err
			}

			result = issuedChallenge{
				challenge: updated,
				auth:      auth,
				issued:    true,
				issueID:   issueID,
				code:      code,
				expiresAt: expiresAt,
			}

			return nil
		})
	})
	if err != nil {
		return issuedChallenge{}, err
	}

	if rejection != nil {
		return issuedChallenge{}, rejection
	}

	return result, nil
}

// sortRateLimitBudgets orders budgets by (Scope, ScopeKey) so ConsumeRateLimit
// acquires row locks in a deterministic order even when authorizers permute
// RateLimits.
func sortRateLimitBudgets(budgets []RateLimitBudget) {
	sort.Slice(budgets, func(i, j int) bool {
		if budgets[i].Scope != budgets[j].Scope {
			return budgets[i].Scope < budgets[j].Scope
		}

		return budgets[i].ScopeKey < budgets[j].ScopeKey
	})
}

func validateAuthorizedIssuance(auth AuthorizedIssuance, input RequestInput) error {
	const op = "otp.validateAuthorizedIssuance"

	if auth.Purpose != input.Purpose {
		return cerr.InternalError(fmt.Errorf("%s: authorizer purpose mismatch", op))
	}

	if auth.SubjectKey != input.SubjectKey {
		return cerr.InternalError(fmt.Errorf("%s: authorizer subject key mismatch", op))
	}

	if auth.ContextVersion != input.ContextVersion {
		return cerr.InternalError(fmt.Errorf("%s: authorizer context version mismatch", op))
	}

	if auth.Binding == "" {
		return cerr.InternalError(fmt.Errorf("%s: binding is required", op))
	}

	if auth.Recipient == "" {
		return cerr.InternalError(fmt.Errorf("%s: recipient is required", op))
	}

	if auth.TemplateID == "" {
		return cerr.InternalError(fmt.Errorf("%s: template id is required", op))
	}

	if auth.IdempotencyKeyPrefix == "" {
		return cerr.InternalError(fmt.Errorf("%s: idempotency key prefix is required", op))
	}

	for i, budget := range auth.RateLimits {
		if !budget.Scope.IsValid() {
			return cerr.InternalError(fmt.Errorf("%s: rate limit[%d] scope is invalid", op, i))
		}

		if budget.ScopeKey == "" {
			return cerr.InternalError(fmt.Errorf("%s: rate limit[%d] scope key is required", op, i))
		}

		if budget.Window <= 0 || budget.MaxSends <= 0 {
			return cerr.InternalError(fmt.Errorf("%s: rate limit[%d] window and max sends must be positive", op, i))
		}
	}

	return nil
}
