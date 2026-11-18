package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// selectExternalCreditKindForStart records only a customer's intent; OTP at
// Confirm remains required to authorize the selected financing offer.
func (uc *UseCase) selectExternalCreditKindForStart(
	ctx context.Context, token model.CheckoutSessionToken, kind model.CreditKind,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.selectExternalCreditKindForStart"
	var selected model.CheckoutSession
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			checkoutSessionQ := uc.checkoutSession.Tx(tx)
			session, err := uc.lockExternalSession(ctx, tx, token)
			if err != nil {
				return err
			}

			if session.Status != model.OpenCheckoutSessionStatus || session.ContractID != nil {
				return checkoutSessionNotContinuableError(fmt.Errorf("checkout status %q", session.Status))
			}

			now := uc.clock.Now().UTC()
			if session.IsExpiredAt(now) {
				return checkoutSessionExpiredError()
			}

			// An omitted kind never silently switches an already-selected
			// monthly product back to the legacy installment product.
			if kind == "" || model.EffectiveCreditKind(session.CreditKind) == kind {
				selected = session
				return nil
			}

			if !kind.IsValid() {
				return validationError("INVALID", "credit_kind", "error.credit_kind_invalid")
			}

			if session.DecisionID != nil {
				return cerr.Conflict(fmt.Errorf("%s: checkout credit kind cannot change after Decision preparation", op))
			}

			productQ, ok := checkoutSessionQ.(repo.CheckoutSessionProductTxQueryer)
			if !ok {
				return cerr.InternalError(fmt.Errorf("%s: product-aware checkout repository unavailable", op))
			}

			selected, err = productQ.SelectCreditKindBeforeDecision(ctx, session.ID, kind, now)
			return err
		})
	})
	return selected, err
}
