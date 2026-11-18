package merchantuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// SetCreditProductEnabled enables or disables a credit product for a merchant.
//
// The operation is an authorized administration command: the transport layer
// must have required permission.MerchantsCreditProductsUpdateAny. It runs in a
// single transaction that locks the merchant profile, so concurrent changes for
// the same merchant serialize.
//
// Semantics:
//   - pay_in_4 keeps legacy compatibility: it is effectively enabled while no
//     explicit row exists. pay_next_month is effectively disabled until it is
//     explicitly enabled here.
//   - Only ACTIVE merchants can be changed; other lifecycle states are rejected
//     with a conflict.
//   - Idempotent: requesting the effective state again performs no write and
//     emits no audit event (Changed=false).
//   - A real change writes the audit outbox message (action
//     merchant_credit_product.set_enabled) in the same transaction.
//
// Existing contracts and Financial Core accounting are not affected.
func (uc *CreditProductUseCase) SetCreditProductEnabled(
	ctx context.Context,
	input SetCreditProductEnabledInput,
) (SetCreditProductEnabledOutput, error) {
	const op = "merchantuc.CreditProductUseCase.SetCreditProductEnabled"

	if input.ActorSubjectID.IsZero() {
		return SetCreditProductEnabledOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: actor subject id is required", op),
		)
	}

	if input.MerchantProfileID.IsZero() {
		return SetCreditProductEnabledOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: merchant profile id is required", op),
		)
	}

	if !input.CreditKind.IsValid() {
		return SetCreditProductEnabledOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: credit kind %q is not supported", op, input.CreditKind),
		)
	}

	now := uc.clock.Now().UTC()
	output := SetCreditProductEnabledOutput{
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
					"%s: merchant status %q does not allow credit product changes", op, profile.Status,
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
		return SetCreditProductEnabledOutput{}, err
	}

	return output, nil
}
