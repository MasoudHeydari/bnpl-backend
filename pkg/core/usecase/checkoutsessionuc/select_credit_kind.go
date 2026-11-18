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
//
// When a previous Decision was a product-scoped denial, the generation is
// bumped and decision_id cleared so the customer can evaluate another eligible
// product without abandoning the merchant checkout.
//
// productChanged is true when the caller requested a different credit_kind than
// the durable session held. START uses that to avoid treating a server-advanced
// evaluation_generation as a stale-client reconfirm failure.
func (uc *UseCase) selectExternalCreditKindForStart(
	ctx context.Context, token model.CheckoutSessionToken, kind model.CreditKind,
) (session model.CheckoutSession, productChanged bool, err error) {
	const op = "checkoutsessionuc.UseCase.selectExternalCreditKindForStart"

	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			checkoutSessionQ := uc.checkoutSession.Tx(tx)
			locked, lockErr := uc.lockExternalSession(ctx, tx, token)
			if lockErr != nil {
				return lockErr
			}

			if locked.Status != model.OpenCheckoutSessionStatus || locked.ContractID != nil {
				return checkoutSessionNotContinuableError(fmt.Errorf("checkout status %q", locked.Status))
			}

			now := uc.clock.Now().UTC()
			if locked.IsExpiredAt(now) {
				return checkoutSessionExpiredError()
			}

			effectiveKind := model.EffectiveCreditKind(locked.CreditKind)
			if kind == "" || effectiveKind == kind {
				session = locked
				return nil
			}

			if !kind.IsValid() {
				return validationError("INVALID", "credit_kind", "error.credit_kind_invalid")
			}

			productChanged = true

			if locked.DecisionID != nil && !locked.DecisionID.IsZero() {
				decisionQ := uc.decisionRepo.Tx(tx)
				decision, getErr := decisionQ.GetByID(ctx, *locked.DecisionID)
				if getErr != nil {
					return getErr
				}

				if decision.Outcome != model.DeniedDecisionOutcome ||
					isSessionTerminalDecisionDenial(decision.ReasonCodes) {
					return cerr.Conflict(fmt.Errorf(
						"%s: checkout credit kind cannot change after Decision preparation",
						op,
					))
				}

				// Clear product-scoped denial in the same TX before selecting.
				bumped, bumpErr := checkoutSessionQ.BumpEvaluationGenerationCAS(
					ctx,
					locked.ID,
					locked.EvaluationGeneration,
					locked.DecisionID,
					now,
				)
				if bumpErr != nil {
					return bumpErr
				}

				locked = bumped
			}

			merchantQ := uc.merchantProfile.Tx(tx)
			enabled, enableErr := merchantQ.IsCreditKindEnabled(ctx, locked.MerchantProfileID, kind)
			if enableErr != nil {
				return enableErr
			}

			if !enabled {
				return validationError("INVALID", "credit_kind", "error.credit_kind_not_enabled")
			}

			productQ, ok := checkoutSessionQ.(repo.CheckoutSessionProductTxQueryer)
			if !ok {
				return cerr.InternalError(fmt.Errorf("%s: product-aware checkout repository unavailable", op))
			}

			selected, selectErr := productQ.SelectCreditKindBeforeDecision(ctx, locked.ID, kind, now)
			if selectErr != nil {
				return selectErr
			}

			session = selected
			return nil
		})
	})
	return session, productChanged, err
}
