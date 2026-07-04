package merchantterminaluc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Retire ends the grace period of a retiring credential of the merchant's
// terminal at once.
//
// Only a retiring credential can be retired. Retiring an already retired
// credential succeeds without change. An active or revoked credential is
// rejected; the active credential is replaced through Rotate. A credential of
// another terminal is reported as not found.
//
// Retire is a merchant operation authenticated through the gateway AuthContext
// (not partner terminal API-key authentication).
func (uc *UseCase) Retire(ctx context.Context, input RetireInput) (RetireOutput, error) {
	const op = "merchantterminaluc.UseCase.Retire"

	if input.MerchantID.IsZero() {
		return RetireOutput{}, merchantIDRequiredError()
	}

	if input.CredentialID.IsZero() {
		return RetireOutput{}, credentialIDRequiredError()
	}

	var output RetireOutput

	err := uc.pool.Conn(
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

					credential, err := terminalQ.GetCredentialByID(ctx, input.CredentialID)
					if err != nil {
						if cerr.IsCode(err, cerr.CodeResourceNotFound) {
							return credentialNotFoundError(err)
						}

						return fmt.Errorf("%s: load credential: %w", op, err)
					}

					if credential.MerchantTerminalID != terminal.ID {
						return credentialNotFoundError(
							fmt.Errorf("%s: credential belongs to another terminal", op),
						)
					}

					switch credential.Status {
					case model.RetiredMerchantTerminalCredentialStatus:
						output = retireOutput(credential)

						return nil

					case model.RetiringMerchantTerminalCredentialStatus:
						now := uc.clock.Now().UTC()

						retired, retireErr := terminalQ.MarkRetired(ctx, credential.ID, now, now)
						if retireErr != nil {
							return fmt.Errorf("%s: mark credential retired: %w", op, retireErr)
						}

						output = retireOutput(retired)

						return nil

					default:
						return credentialNotRetirableError(
							fmt.Errorf("%s: credential status is %q", op, credential.Status),
						)
					}
				},
			)
		},
	)
	if err != nil {
		return RetireOutput{}, err
	}

	return output, nil
}

func retireOutput(credential model.MerchantTerminalCredential) RetireOutput {
	output := RetireOutput{
		CredentialID: credential.ID,
		Version:      credential.Version,
	}

	if credential.RetiredAt != nil {
		output.RetiredAt = *credential.RetiredAt
	}

	return output
}
