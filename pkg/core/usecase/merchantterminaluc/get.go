package merchantterminaluc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Get returns the authenticated merchant's terminals without any API key secret.
//
// Show-once policy: plaintext API keys are never included here. BNPL stores only
// an irreversible verifier. The plaintext key is returned only by the initial
// issuance response (Provision/Rotate) and by an idempotent replay of that same
// issuance request within the configured replay window. Outside that window a
// lost key requires Rotate.
//
// A merchant currently owns at most one terminal. When none exists, Get returns
// an empty Items slice with no error so the panel can show an empty state
// instead of treating absence as a client/server failure. Active and
// still-acceptable retiring credentials are included as metadata only.
func (uc *UseCase) Get(ctx context.Context, input GetInput) (GetOutput, error) {
	const op = "merchantterminaluc.UseCase.Get"

	if input.MerchantID.IsZero() {
		return GetOutput{}, merchantIDRequiredError()
	}

	output := GetOutput{Items: []GetTerminal{}}

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			terminalQ := uc.merchantTerminal.Conn(conn)

			terminal, err := terminalQ.GetByMerchantID(ctx, input.MerchantID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return nil
				}

				return fmt.Errorf("%s: load merchant terminal: %w", op, err)
			}

			now := uc.clock.Now().UTC()

			credentials, err := terminalQ.ListAcceptableCredentials(ctx, terminal.ID, now)
			if err != nil {
				return fmt.Errorf("%s: list acceptable credentials: %w", op, err)
			}

			item := GetTerminal{
				MerchantID: terminal.MerchantID,
				TerminalID: terminal.TerminalID,
				Status:     terminal.Status,
				CreatedAt:  terminal.CreatedAt,
				UpdatedAt:  terminal.UpdatedAt,
			}

			for _, credential := range credentials {
				switch credential.Status {
				case model.ActiveMerchantTerminalCredentialStatus:
					item.ActiveCredential = &GetActiveCredential{
						CredentialID: credential.ID,
						Version:      credential.Version,
						ActivatedAt:  credential.ActivatedAt,
					}

				case model.RetiringMerchantTerminalCredentialStatus:
					if !credential.IsAcceptableAt(now) {
						continue
					}

					item.Retiring = &RetiringCredential{
						CredentialID: credential.ID,
						Version:      credential.Version,
						RetireAt:     *credential.RetireAt,
					}

				default:
					// ListAcceptableCredentials only returns active and retiring.
				}
			}

			output.Items = []GetTerminal{item}

			return nil
		},
	)
	if err != nil {
		return GetOutput{}, err
	}

	return output, nil
}
