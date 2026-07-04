package decisionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func isPlausibleIdempotencyCreateConflict(err error) bool {
	if err == nil {
		return false
	}

	return cerr.IsCode(err, cerr.CodeValidationFailed) || cerr.IsCode(err, cerr.CodeConflict)
}

func (uc *UseCase) resolveIdempotentDecision(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	allocationQ repo.CheckoutDecisionAllocationTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	idempotencyKey string,
	fingerprint string,
	input CreateInput,
) (CreateOutput, bool, error) {
	const op = "decisionuc.UseCase.resolveIdempotentDecision"

	existing, err := decisionQ.GetByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return CreateOutput{}, false, nil
		}

		return CreateOutput{}, false, err
	}

	if existing.IdempotencyFingerprint != fingerprint {
		return CreateOutput{}, true, idempotencyConflictError(
			fmt.Errorf("%s: idempotency key reused with different checkout inputs", op),
		)
	}

	output, err := uc.outputFromExistingDecision(
		ctx,
		allocationQ,
		installmentQ,
		installmentChargeQ,
		existing,
		input,
	)

	return output, true, err
}

func (uc *UseCase) resolveCheckoutDecisionAfterConflict(
	ctx context.Context,
	input CreateInput,
	fingerprint string,
) (CreateOutput, error) {
	var output CreateOutput
	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					decisionQ := uc.decision.Tx(tx)
					allocationQ := uc.checkoutDecisionAllocation.Tx(tx)
					installmentQ := uc.installment.Tx(tx)
					installmentChargeQ := uc.installmentCharge.Tx(tx)
					resolvedOutput, resolved, resolveErr := uc.resolveIdempotentDecision(
						ctx,
						decisionQ,
						allocationQ,
						installmentQ,
						installmentChargeQ,
						input.CheckoutKey,
						fingerprint,
						input,
					)
					if resolveErr != nil {
						return resolveErr
					}

					if !resolved {
						return checkoutDecisionCreateUnresolvedConflictError()
					}

					output = resolvedOutput

					return nil
				},
			)
		},
	)
	if err != nil {
		return CreateOutput{}, err
	}

	return output, nil
}
