package merchantterminaluc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Provision creates the merchant's terminal: it generates the public
// terminal_id, stores the v1 credential, persists initial server-side
// permission assignments, and issues the first plaintext API key.
// The merchant must be active and may own only one terminal.
//
// Provision is idempotent per merchant and IdempotencyKey: a retry within
// Config.IssuanceDeliveryTTL returns the same terminal and API key while that
// credential is still ACTIVE. Reusing the key after the replay window, after the
// credential was superseded by a rotation, or after the secret was purged is an
// idempotency conflict, and a key bound to a different request fingerprint is
// always a conflict. Each attempt runs in its own transaction because a unique
// violation aborts the PostgreSQL transaction that hit it, so a terminal id
// collision cannot be retried inside the same one.
func (uc *UseCase) Provision(ctx context.Context, input ProvisionInput) (ProvisionOutput, error) {
	const op = "merchantterminaluc.UseCase.Provision"

	if input.MerchantID.IsZero() {
		return ProvisionOutput{}, merchantIDRequiredError()
	}

	err := validateIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		return ProvisionOutput{}, err
	}

	var lastRetryable error

	for range provisionMaxAttempts {
		output, attemptErr := uc.provisionOnce(ctx, input)
		if attemptErr == nil {
			return output, nil
		}

		if !isRetryableProvisionError(attemptErr) {
			return ProvisionOutput{}, attemptErr
		}

		lastRetryable = attemptErr
	}

	return ProvisionOutput{}, provisionContendedError(
		fmt.Errorf("%s: gave up after %d attempts: %w", op, provisionMaxAttempts, lastRetryable),
	)
}

func isRetryableProvisionError(err error) bool {
	return errors.Is(err, repo.ErrMerchantTerminalIDTaken) ||
		errors.Is(err, repo.ErrMerchantTerminalExists)
}

func (uc *UseCase) provisionOnce(ctx context.Context, input ProvisionInput) (ProvisionOutput, error) {
	const op = "merchantterminaluc.UseCase.provisionOnce"

	var output ProvisionOutput

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					profileQ := uc.merchantProfile.Tx(tx)

					profile, err := profileQ.GetByMerchantID(ctx, input.MerchantID)
					if err != nil {
						if cerr.IsCode(err, cerr.CodeResourceNotFound) {
							return merchantProfileNotFoundError(err)
						}

						return fmt.Errorf("%s: load merchant profile: %w", op, err)
					}

					if !profile.Status.IsActive() {
						return merchantNotActiveError(
							fmt.Errorf("%s: merchant status is %q", op, profile.Status),
						)
					}

					terminalQ := uc.merchantTerminal.Tx(tx)
					now := uc.clock.Now().UTC()
					storedKey := provisionKeyPrefix + input.IdempotencyKey
					fingerprint := provisionFingerprint(input.MerchantID)

					existing, err := terminalQ.LockByMerchantID(ctx, input.MerchantID)
					switch {
					case err == nil:
						replayed, replayErr := uc.replayProvision(
							ctx,
							terminalQ,
							existing,
							storedKey,
							fingerprint,
							now,
						)
						if replayErr != nil {
							return replayErr
						}

						output = replayed

						return nil

					case cerr.IsCode(err, cerr.CodeResourceNotFound):
						created, createErr := uc.createTerminal(
							ctx,
							terminalQ,
							input.MerchantID,
							storedKey,
							fingerprint,
							now,
						)
						if createErr != nil {
							return createErr
						}

						output = created

						return nil

					default:
						return fmt.Errorf("%s: lock merchant terminal: %w", op, err)
					}
				},
			)
		},
	)
	if err != nil {
		return ProvisionOutput{}, err
	}

	return output, nil
}

// replayProvision answers a retry for a merchant that already owns a terminal.
// Only the request that created the terminal can replay; any other key means
// the merchant is already provisioned, and a key bound to a different request
// fingerprint is an idempotency conflict.
func (uc *UseCase) replayProvision(
	ctx context.Context,
	terminalQ repo.MerchantTerminalTxQueryer,
	terminal model.MerchantTerminal,
	storedKey string,
	fingerprint string,
	now time.Time,
) (ProvisionOutput, error) {
	const op = "merchantterminaluc.UseCase.replayProvision"

	if !terminal.Status.IsActive() {
		return ProvisionOutput{}, terminalNotActiveError(
			fmt.Errorf("%s: terminal status is %q", op, terminal.Status),
		)
	}

	delivery, err := terminalQ.GetIssuanceDeliveryByIdempotencyKey(ctx, terminal.ID, storedKey)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return ProvisionOutput{}, terminalAlreadyProvisionedError(
				fmt.Errorf("%s: no delivery for idempotency key", op),
			)
		}

		return ProvisionOutput{}, fmt.Errorf("%s: load issuance delivery: %w", op, err)
	}

	if delivery.RequestFingerprint != fingerprint {
		return ProvisionOutput{}, idempotencyConflictError(
			fmt.Errorf("%s: idempotency key was used for a different request", op),
		)
	}

	replayed, err := uc.replayIssuance(ctx, terminalQ, delivery, now)
	if err != nil {
		return ProvisionOutput{}, err
	}

	return ProvisionOutput{
		MerchantID:        terminal.MerchantID,
		TerminalID:        terminal.TerminalID,
		CredentialID:      replayed.credential.ID,
		CredentialVersion: replayed.credential.Version,
		APIKey:            replayed.plaintext,
		CreatedAt:         replayed.credential.CreatedAt,
		ExpiresAt:         replayed.expiresAt,
		Replayed:          true,
	}, nil
}

// createTerminal stores a new terminal with its first credential and the
// encrypted issuance delivery.
func (uc *UseCase) createTerminal(
	ctx context.Context,
	terminalQ repo.MerchantTerminalTxQueryer,
	merchantID model.MerchantID,
	storedKey string,
	fingerprint string,
	now time.Time,
) (ProvisionOutput, error) {
	const op = "merchantterminaluc.UseCase.createTerminal"

	terminalID, err := model.GenerateTerminalID(uc.random)
	if err != nil {
		return ProvisionOutput{}, cerr.InternalError(fmt.Errorf("%s: generate terminal id: %w", op, err))
	}

	key, err := uc.newIssuedKey()
	if err != nil {
		return ProvisionOutput{}, err
	}

	terminal, err := terminalQ.CreateTerminal(
		ctx,
		merchantID,
		terminalID,
		model.ActiveMerchantTerminalStatus,
		now,
		now,
	)
	if err != nil {
		return ProvisionOutput{}, fmt.Errorf("%s: create terminal: %w", op, err)
	}

	credential, err := terminalQ.CreateCredential(
		ctx,
		terminal.ID,
		1,
		key.hash,
		model.ActiveMerchantTerminalCredentialStatus,
		now,
		now,
		now,
	)
	if err != nil {
		return ProvisionOutput{}, fmt.Errorf("%s: create credential: %w", op, err)
	}

	err = terminalQ.ReplacePermissions(
		ctx,
		terminal.ID,
		merchantterminal.InitialPartnerTerminalPermissions(),
		now,
	)
	if err != nil {
		return ProvisionOutput{}, fmt.Errorf("%s: assign terminal permissions: %w", op, err)
	}

	expiresAt := uc.clock.Add(now, uc.cfg.IssuanceDeliveryTTL)

	delivery, err := terminalQ.CreateIssuanceDelivery(
		ctx,
		terminal.ID,
		credential.ID,
		storedKey,
		fingerprint,
		key.ciphertext,
		key.encryptionID,
		expiresAt,
		now,
	)
	if err != nil {
		return ProvisionOutput{}, fmt.Errorf("%s: create issuance delivery: %w", op, err)
	}

	_, err = terminalQ.ConsumeIssuanceDelivery(ctx, delivery.ID, now)
	if err != nil {
		return ProvisionOutput{}, fmt.Errorf("%s: consume issuance delivery: %w", op, err)
	}

	return ProvisionOutput{
		MerchantID:        terminal.MerchantID,
		TerminalID:        terminal.TerminalID,
		CredentialID:      credential.ID,
		CredentialVersion: credential.Version,
		APIKey:            key.plaintext,
		CreatedAt:         credential.CreatedAt,
		ExpiresAt:         expiresAt,
		Replayed:          false,
	}, nil
}
