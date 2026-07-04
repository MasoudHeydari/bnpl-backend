package merchantterminaluc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Rotate issues a new API key for the merchant's terminal and keeps the
// previous one accepted for the grace period.
//
// It refuses while a previous rotation is still inside its grace period. A
// previous credential that is past its deadline is logically retired already
// and is marked retired in the same transaction. A grace period of zero retires
// the previous credential immediately (RETIRED, never RETIRING), so the output
// carries no Retiring credential.
//
// Retries with the same IdempotencyKey and the same effective grace period
// replay the new key while its credential is still ACTIVE and the delivery has
// not expired. The effective grace is the one after Config.DefaultGrace was
// applied, so omitting it and passing the default explicitly are the same
// request. The same key with a different effective grace is an idempotency
// conflict. A replay after a later rotation superseded the credential is
// rejected as stale.
//
// Rotate is a merchant operation authenticated through the gateway AuthContext
// (not partner terminal API-key authentication).
func (uc *UseCase) Rotate(ctx context.Context, input RotateInput) (RotateOutput, error) {
	const op = "merchantterminaluc.UseCase.Rotate"

	if input.MerchantID.IsZero() {
		return RotateOutput{}, merchantIDRequiredError()
	}

	err := validateIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		return RotateOutput{}, err
	}

	grace, err := uc.resolveGrace(input.GracePeriod)
	if err != nil {
		return RotateOutput{}, err
	}

	fingerprint := rotateFingerprint(input.MerchantID, grace)

	var output RotateOutput

	err = uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					terminalQ := uc.merchantTerminal.Tx(tx)

					terminal, err := terminalQ.LockByMerchantID(ctx, input.MerchantID)
					if err != nil {
						if cerr.IsCode(err, cerr.CodeResourceNotFound) {
							return terminalNotFoundError(err)
						}

						return fmt.Errorf("%s: lock merchant terminal: %w", op, err)
					}

					if !terminal.Status.IsActive() {
						return terminalNotActiveError(
							fmt.Errorf("%s: terminal status is %q", op, terminal.Status),
						)
					}

					now := uc.clock.Now().UTC()
					storedKey := rotateKeyPrefix + input.IdempotencyKey

					credentials, err := terminalQ.LockCredentialsForRotation(ctx, terminal.ID)
					if err != nil {
						return fmt.Errorf("%s: lock credentials: %w", op, err)
					}

					delivery, err := terminalQ.GetIssuanceDeliveryByIdempotencyKey(ctx, terminal.ID, storedKey)
					switch {
					case err == nil:
						replayed, replayErr := uc.replayRotation(
							ctx,
							terminalQ,
							terminal,
							delivery,
							credentials,
							fingerprint,
							now,
						)
						if replayErr != nil {
							return replayErr
						}

						output = replayed

						return nil

					case !cerr.IsCode(err, cerr.CodeResourceNotFound):
						return fmt.Errorf("%s: load issuance delivery: %w", op, err)
					}

					rotated, err := uc.rotateCredentials(
						ctx,
						terminalQ,
						terminal,
						credentials,
						storedKey,
						fingerprint,
						grace,
						now,
					)
					if err != nil {
						return err
					}

					output = rotated

					return nil
				},
			)
		},
	)
	if err != nil {
		return RotateOutput{}, err
	}

	return output, nil
}

// resolveGrace applies the default and validates the requested grace period.
// The returned value is the effective grace used for the request fingerprint.
func (uc *UseCase) resolveGrace(requested *time.Duration) (time.Duration, error) {
	if requested == nil {
		return uc.cfg.DefaultGrace, nil
	}

	if *requested < 0 {
		return 0, gracePeriodInvalidError()
	}

	if *requested > uc.cfg.MaxGrace {
		return 0, gracePeriodTooLongError()
	}

	return *requested, nil
}

// replayRotation answers a retry of a rotation that already succeeded. A
// different request fingerprint under the same key is an idempotency conflict.
func (uc *UseCase) replayRotation(
	ctx context.Context,
	terminalQ repo.MerchantTerminalTxQueryer,
	terminal model.MerchantTerminal,
	delivery model.MerchantTerminalIssuanceDelivery,
	credentials []model.MerchantTerminalCredential,
	fingerprint string,
	now time.Time,
) (RotateOutput, error) {
	const op = "merchantterminaluc.UseCase.replayRotation"

	if delivery.RequestFingerprint != fingerprint {
		return RotateOutput{}, idempotencyConflictError(
			fmt.Errorf("%s: idempotency key was used for a different request", op),
		)
	}

	replayed, err := uc.replayIssuance(ctx, terminalQ, delivery, now)
	if err != nil {
		return RotateOutput{}, err
	}

	return RotateOutput{
		MerchantID:        terminal.MerchantID,
		TerminalID:        terminal.TerminalID,
		CredentialID:      replayed.credential.ID,
		CredentialVersion: replayed.credential.Version,
		APIKey:            replayed.plaintext,
		ActivatedAt:       replayed.credential.ActivatedAt,
		Retiring:          retiringBelow(credentials, replayed.credential.Version, now),
		ExpiresAt:         replayed.expiresAt,
		Replayed:          true,
	}, nil
}

// retiringBelow returns the still-acceptable retiring credential older than
// version, if any. Credentials whose retire_at has already passed are omitted
// so a delayed DB normalization cannot present them as currently usable.
func retiringBelow(
	credentials []model.MerchantTerminalCredential,
	version int64,
	now time.Time,
) *RetiringCredential {
	for _, credential := range credentials {
		if credential.Status != model.RetiringMerchantTerminalCredentialStatus ||
			credential.RetireAt == nil ||
			credential.Version >= version ||
			!credential.IsAcceptableAt(now) {
			continue
		}

		return &RetiringCredential{
			CredentialID: credential.ID,
			Version:      credential.Version,
			RetireAt:     *credential.RetireAt,
		}
	}

	return nil
}

// rotateCredentials performs a new rotation inside the locked terminal.
func (uc *UseCase) rotateCredentials(
	ctx context.Context,
	terminalQ repo.MerchantTerminalTxQueryer,
	terminal model.MerchantTerminal,
	credentials []model.MerchantTerminalCredential,
	storedKey string,
	fingerprint string,
	grace time.Duration,
	now time.Time,
) (RotateOutput, error) {
	const op = "merchantterminaluc.UseCase.rotateCredentials"

	var active *model.MerchantTerminalCredential

	for index := range credentials {
		credential := credentials[index]

		switch credential.Status {
		case model.ActiveMerchantTerminalCredentialStatus:
			active = &credentials[index]

		case model.RetiringMerchantTerminalCredentialStatus:
			if credential.IsAcceptableAt(now) {
				return RotateOutput{}, credentialRotationInProgressError(
					fmt.Errorf("%s: credential version %d is still retiring", op, credential.Version),
				)
			}

			// Past its deadline the credential is already rejected by
			// authentication; record that so a later rotation starts clean.
			// retired_at is the effective grace deadline, not this later
			// normalization time.
			retiredAt := *credential.RetireAt
			_, err := terminalQ.MarkRetired(ctx, credential.ID, retiredAt, now)
			if err != nil {
				return RotateOutput{}, fmt.Errorf("%s: retire expired credential: %w", op, err)
			}

		default:
			// LockCredentialsForRotation only returns active and retiring rows.
		}
	}

	if active == nil {
		return RotateOutput{}, cerr.InternalError(
			fmt.Errorf("%s: terminal has no active credential", op),
		)
	}

	key, err := uc.newIssuedKey()
	if err != nil {
		return RotateOutput{}, err
	}

	// A zero grace retires the previous credential at once; it must never be
	// recorded as RETIRING with a deadline equal to now.
	var retiring *RetiringCredential

	if grace == 0 {
		_, err = terminalQ.MarkRetired(ctx, active.ID, now, now)
		if err != nil {
			return RotateOutput{}, fmt.Errorf("%s: mark credential retired: %w", op, err)
		}
	} else {
		retireAt := uc.clock.Add(now, grace)

		marked, markErr := terminalQ.MarkRetiring(ctx, active.ID, retireAt, now)
		if markErr != nil {
			return RotateOutput{}, fmt.Errorf("%s: mark credential retiring: %w", op, markErr)
		}

		retiring = &RetiringCredential{
			CredentialID: marked.ID,
			Version:      marked.Version,
			RetireAt:     retireAt,
		}
	}

	created, err := terminalQ.CreateCredential(
		ctx,
		terminal.ID,
		active.Version+1,
		key.hash,
		model.ActiveMerchantTerminalCredentialStatus,
		now,
		now,
		now,
	)
	if err != nil {
		return RotateOutput{}, fmt.Errorf("%s: create credential: %w", op, err)
	}

	expiresAt := uc.clock.Add(now, uc.cfg.IssuanceDeliveryTTL)

	delivery, err := terminalQ.CreateIssuanceDelivery(
		ctx,
		terminal.ID,
		created.ID,
		storedKey,
		fingerprint,
		key.ciphertext,
		key.encryptionID,
		expiresAt,
		now,
	)
	if err != nil {
		return RotateOutput{}, fmt.Errorf("%s: create issuance delivery: %w", op, err)
	}

	_, err = terminalQ.ConsumeIssuanceDelivery(ctx, delivery.ID, now)
	if err != nil {
		return RotateOutput{}, fmt.Errorf("%s: consume issuance delivery: %w", op, err)
	}

	return RotateOutput{
		MerchantID:        terminal.MerchantID,
		TerminalID:        terminal.TerminalID,
		CredentialID:      created.ID,
		CredentialVersion: created.Version,
		APIKey:            key.plaintext,
		ActivatedAt:       created.ActivatedAt,
		Retiring:          retiring,
		ExpiresAt:         expiresAt,
		Replayed:          false,
	}, nil
}
