package merchantterminaluc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// issuedKey is a freshly generated API key in every form the use case needs.
// It must never be logged or embedded in an error.
type issuedKey struct {
	plaintext    string
	hash         string
	ciphertext   string
	encryptionID string
}

// newIssuedKey generates an API key, its storable hash, and its encrypted
// replay copy.
func (uc *UseCase) newIssuedKey() (issuedKey, error) {
	const op = "merchantterminaluc.UseCase.newIssuedKey"

	plaintext, err := merchantterminal.GenerateAPIKey(uc.random)
	if err != nil {
		return issuedKey{}, cerr.InternalError(fmt.Errorf("%s: generate api key: %w", op, err))
	}

	value, err := uc.hasher.Hash(plaintext)
	if err != nil {
		return issuedKey{}, cerr.InternalError(fmt.Errorf("%s: hash api key: %w", op, err))
	}

	ciphertext, encryptionID, err := uc.box.Encrypt(plaintext)
	if err != nil {
		return issuedKey{}, cerr.InternalError(fmt.Errorf("%s: encrypt api key: %w", op, err))
	}

	return issuedKey{
		plaintext:    plaintext,
		hash:         value.String(),
		ciphertext:   ciphertext,
		encryptionID: encryptionID,
	}, nil
}

// replayedIssuance is the result of replaying a stored issuance delivery.
type replayedIssuance struct {
	plaintext  string
	credential model.MerchantTerminalCredential
	expiresAt  time.Time
}

// replayIssuance decrypts the API key of delivery for an idempotent retry and
// records the delivery as consumed.
//
// The replay is refused when the secret was purged, the delivery expired, or
// its credential is no longer ACTIVE. A credential that was superseded by a
// later rotation is stale: returning its key would hand the caller a secret
// that is retiring or dead. Callers must check the request fingerprint before
// calling so a different request under the same key is a conflict, not a
// replay.
//
// consumed_at only records the first extraction; it never blocks a later
// in-window replay.
func (uc *UseCase) replayIssuance(
	ctx context.Context,
	terminalQ repo.MerchantTerminalTxQueryer,
	delivery model.MerchantTerminalIssuanceDelivery,
	now time.Time,
) (replayedIssuance, error) {
	const op = "merchantterminaluc.UseCase.replayIssuance"

	if !delivery.HasReplayableSecret() {
		return replayedIssuance{}, issuanceReplayUnavailableError(
			fmt.Errorf("%s: delivery secret was purged", op),
		)
	}

	if !now.Before(delivery.ExpiresAt) {
		return replayedIssuance{}, issuanceReplayUnavailableError(
			fmt.Errorf("%s: delivery expired", op),
		)
	}

	credential, err := terminalQ.GetCredentialByID(ctx, delivery.CredentialID)
	if err != nil {
		return replayedIssuance{}, fmt.Errorf("%s: load credential: %w", op, err)
	}

	if credential.Status != model.ActiveMerchantTerminalCredentialStatus {
		return replayedIssuance{}, issuanceReplayUnavailableError(
			fmt.Errorf("%s: credential status is %q, not active", op, credential.Status),
		)
	}

	plaintext, err := uc.box.Decrypt(delivery.APIKeyCiphertext, delivery.APIKeyEncryptionKeyID)
	if err != nil {
		return replayedIssuance{}, cerr.InternalError(fmt.Errorf("%s: decrypt api key: %w", op, err))
	}

	_, err = terminalQ.ConsumeIssuanceDelivery(ctx, delivery.ID, now)
	if err != nil {
		return replayedIssuance{}, fmt.Errorf("%s: consume issuance delivery: %w", op, err)
	}

	return replayedIssuance{
		plaintext:  plaintext,
		credential: credential,
		expiresAt:  delivery.ExpiresAt,
	}, nil
}

func validateIdempotencyKey(key string) error {
	if key == "" {
		return idempotencyKeyRequiredError()
	}

	if len(key) > maxIdempotencyKeyLength {
		return idempotencyKeyTooLongError()
	}

	return nil
}
