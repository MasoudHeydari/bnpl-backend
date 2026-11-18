package merchantuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// SetCreditKindEnabled enables or disables a credit kind for a merchant.
//
// The operation is an authorized administration command: the transport layer
// must have required permission.MerchantsCreditProductsUpdateAny. It runs in a
// single transaction that locks the merchant profile, so concurrent changes for
// the same merchant serialize.
//
// Semantics:
//   - Both credit kinds are disabled until explicitly enabled here.
//   - Only ACTIVE merchants can be changed; other lifecycle states are rejected
//     with a conflict.
//   - Idempotent: requesting the effective state again performs no write and
//     emits no audit event (Changed=false).
//   - A real change writes the audit outbox message (action
//     merchant_credit_kind.set_enabled) in the same transaction.
//
// Existing contracts and Financial Core accounting are not affected.
func (uc *CreditKindUseCase) SetCreditKindEnabled(
	ctx context.Context,
	input SetCreditKindEnabledInput,
) (SetCreditKindEnabledOutput, error) {
	const op = "merchantuc.CreditKindUseCase.SetCreditKindEnabled"

	if input.ActorSubjectID.IsZero() {
		return SetCreditKindEnabledOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: actor subject id is required", op),
		)
	}

	if input.MerchantProfileID.IsZero() {
		return SetCreditKindEnabledOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: merchant profile id is required", op),
		)
	}

	if !input.CreditKind.IsValid() {
		return SetCreditKindEnabledOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: credit kind %q is not supported", op, input.CreditKind),
		)
	}

	now := uc.clock.Now().UTC()
	output := SetCreditKindEnabledOutput{
		MerchantProfileID: input.MerchantProfileID,
		CreditKind:        input.CreditKind,
		Enabled:           input.Enabled,
	}

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			merchantProfileQ := uc.merchantProfile.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			profile, err := merchantProfileQ.LockByID(ctx, input.MerchantProfileID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return merchantProfileNotFoundError(err)
				}

				return fmt.Errorf("%s: lock merchant profile: %w", op, err)
			}

			if !profile.Status.IsActive() {
				return cerr.Conflict(fmt.Errorf(
					"%s: merchant status %q does not allow credit kind changes", op, profile.Status,
				))
			}

			previous, err := merchantProfileQ.IsCreditKindEnabled(ctx, profile.ID, input.CreditKind)
			if err != nil {
				return fmt.Errorf("%s: read current eligibility: %w", op, err)
			}

			if previous == input.Enabled {
				return nil
			}

			err = merchantProfileQ.SetCreditKindEnabled(ctx, profile.ID, input.CreditKind, input.Enabled, now)
			if err != nil {
				return fmt.Errorf("%s: set eligibility: %w", op, err)
			}

			output.Changed = true

			err = uc.insertSetEnabledAuditOutbox(ctx, outboxQ, profile, input, previous, now)
			if err != nil {
				return fmt.Errorf("%s: record audit: %w", op, err)
			}

			return nil
		})
	})
	if err != nil {
		return SetCreditKindEnabledOutput{}, err
	}

	return output, nil
}
