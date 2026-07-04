package merchantterminaluc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Authenticate resolves a presented terminal id and API key to a principal.
//
// Permissions are loaded from server-side merchant_terminal_permissions for the
// authenticated terminal. They are never taken from partner headers, the API
// key, or a runtime hard-coded default. Unknown stored permission strings are
// dropped (never granted). A repository failure fails closed as an internal
// error.
//
// Every rejection of the presented credentials returns the same public
// unauthorized error. The API key, its hash, and pepper values are never logged
// or placed in a public error. Missing referenced pepper keys still yield that
// public unauthorized shape; the wrapped cause identifies the pepper key id for
// operators.
func (uc *UseCase) Authenticate(
	ctx context.Context,
	terminalID model.TerminalID,
	apiKey string,
) (merchantterminal.MerchantPrincipal, error) {
	const op = "merchantterminaluc.UseCase.Authenticate"

	if terminalID <= 0 {
		return merchantterminal.MerchantPrincipal{}, invalidCredentialsError(
			fmt.Errorf("%s: terminal id is not positive", op),
		)
	}

	formatErr := merchantterminal.ValidateAPIKeyFormat(apiKey)
	if formatErr != nil {
		return merchantterminal.MerchantPrincipal{}, invalidCredentialsError(
			fmt.Errorf("%s: api key format: %w", op, formatErr),
		)
	}

	var principal merchantterminal.MerchantPrincipal

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			terminalQ := uc.merchantTerminal.Conn(conn)

			terminal, err := terminalQ.GetByTerminalID(ctx, terminalID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					uc.spendComparisonWork(apiKey)

					return invalidCredentialsError(fmt.Errorf("%s: terminal not found", op))
				}

				return fmt.Errorf("%s: load terminal: %w", op, err)
			}

			if !terminal.Status.IsActive() {
				uc.spendComparisonWork(apiKey)

				return invalidCredentialsError(fmt.Errorf("%s: terminal is not active", op))
			}

			now := uc.clock.Now().UTC()

			credentials, err := terminalQ.ListAcceptableCredentials(ctx, terminal.ID, now)
			if err != nil {
				return fmt.Errorf("%s: list acceptable credentials: %w", op, err)
			}

			matched, err := uc.matchCredential(apiKey, credentials, now)
			if err != nil {
				if errors.Is(err, fingerprint.ErrPepperKeyMissing) {
					return invalidCredentialsError(fmt.Errorf(
						"%s: credential_id=%s terminal_id=%s: %w",
						op,
						matched.ID,
						terminal.TerminalID,
						err,
					))
				}

				return invalidCredentialsError(fmt.Errorf("%s: api key did not match: %w", op, err))
			}

			storedPermissions, err := terminalQ.ListPermissions(ctx, terminal.ID)
			if err != nil {
				return fmt.Errorf("%s: list terminal permissions: %w", op, err)
			}

			principal = merchantterminal.MerchantPrincipal{
				MerchantID:        terminal.MerchantID,
				TerminalID:        terminal.TerminalID,
				CredentialID:      matched.ID,
				CredentialVersion: matched.Version,
				Permissions:       permission.FilterKnown(storedPermissions),
			}

			return nil
		},
	)
	if err != nil {
		return merchantterminal.MerchantPrincipal{}, err
	}

	return principal, nil
}

// matchCredential compares apiKey against every acceptable credential. It does
// not stop at the first match so timing does not reveal the matching position.
//
// A missing pepper key for a stored verifier returns ErrPepperKeyMissing so
// callers can record an internal diagnostic while still mapping to a generic
// unauthorized public error.
func (uc *UseCase) matchCredential(
	apiKey string,
	credentials []model.MerchantTerminalCredential,
	now time.Time,
) (model.MerchantTerminalCredential, error) {
	var (
		matched     model.MerchantTerminalCredential
		found       bool
		pepperError error
	)

	for _, credential := range credentials {
		if !credential.IsAcceptableAt(now) {
			continue
		}

		ok, err := uc.hasher.Verify(apiKey, credential.APIKeyHash)
		if err != nil {
			if errors.Is(err, fingerprint.ErrPepperKeyMissing) && pepperError == nil {
				pepperError = err
				matched = credential
			}

			continue
		}

		if ok && !found {
			matched = credential
			found = true
		}
	}

	if found {
		return matched, nil
	}

	if pepperError != nil {
		return matched, pepperError
	}

	return model.MerchantTerminalCredential{}, fmt.Errorf("no matching credential")
}

// spendComparisonWork hashes the key so a rejected terminal costs about as much
// as a rejected key. The result is intentionally discarded.
func (uc *UseCase) spendComparisonWork(apiKey string) {
	_, _ = uc.hasher.Hash(apiKey)
}
