package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// SelectCreditKindInput is the Partner continuation that records the
// customer-chosen credit kind after the merchant-site selection UI.
type SelectCreditKindInput struct {
	// Token is the public checkout session token returned by CreatePartner.
	Token model.CheckoutSessionToken
	// ExternalMerchantID must match the terminal-authenticated merchant.
	ExternalMerchantID model.ExternalMerchantID
	// CreditKind is the customer-selected kind (pay_in_4 or pay_next_month).
	CreditKind model.CreditKind
}

// SelectCreditKindOutput returns the session after selection and, when
// preparation mode is on_create, the prepared Offer.
type SelectCreditKindOutput struct {
	Session model.CheckoutSession
	Offer   *Offer
}

func (i SelectCreditKindInput) validate() error {
	if err := validateCheckoutSessionToken(i.Token); err != nil {
		return err
	}

	if i.ExternalMerchantID.IsZero() {
		return validationError("REQUIRED", "external_merchant_id", "error.merchant_id_required")
	}

	if !i.CreditKind.IsValid() {
		return validationError("INVALID", "credit_kind", "error.credit_kind_invalid")
	}

	return nil
}

// SelectCreditKind persists the merchant-communicated credit-kind selection.
// Hosted START must not override this value. Idempotent when the same kind is
// already selected. After selection, on_create sessions prepare the offer.
func (uc *UseCase) SelectCreditKind(
	ctx context.Context,
	input SelectCreditKindInput,
) (SelectCreditKindOutput, error) {
	const op = "checkoutsessionuc.UseCase.SelectCreditKind"

	if err := input.validate(); err != nil {
		return SelectCreditKindOutput{}, err
	}

	session, _, err := uc.selectPartnerCreditKind(ctx, input.Token, input.ExternalMerchantID, input.CreditKind)
	if err != nil {
		return SelectCreditKindOutput{}, err
	}

	output := SelectCreditKindOutput{Session: session}

	if session.EffectiveOfferPreparationMode() != model.OfferPreparationOnCreate {
		return output, nil
	}

	uc.observer.OfferPreparationStarted(ctx, session.ID)

	prepared, offer, prepErr := uc.prepareExternalOffer(ctx, session)
	if prepErr != nil {
		uc.observer.OfferPreparationFailed(ctx, session.ID, "offer_preparation_failed")
		return SelectCreditKindOutput{}, prepErr
	}

	output.Session = prepared
	if offer.Outcome != "" {
		output.Offer = &offer

		var decisionID model.DecisionID
		if prepared.DecisionID != nil {
			decisionID = *prepared.DecisionID
		}

		uc.observer.OfferPrepared(ctx, prepared.ID, decisionID)
	}

	return output, nil
}

func (uc *UseCase) selectPartnerCreditKind(
	ctx context.Context,
	token model.CheckoutSessionToken,
	externalMerchantID model.ExternalMerchantID,
	kind model.CreditKind,
) (session model.CheckoutSession, changed bool, err error) {
	const op = "checkoutsessionuc.UseCase.selectPartnerCreditKind"

	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			checkoutSessionQ := uc.checkoutSession.Tx(tx)
			locked, lockErr := uc.lockExternalSession(ctx, tx, token)
			if lockErr != nil {
				return lockErr
			}

			merchantQ := uc.merchantProfile.Tx(tx)
			merchant, merchantErr := merchantQ.GetByID(ctx, locked.MerchantProfileID)
			if merchantErr != nil {
				return merchantErr
			}

			if merchant.ExternalMerchantID != externalMerchantID {
				return checkoutSessionNotFoundError(fmt.Errorf(
					"%s: checkout does not belong to merchant", op,
				))
			}

			if locked.Status != model.OpenCheckoutSessionStatus || locked.ContractID != nil {
				return checkoutSessionNotContinuableError(fmt.Errorf("checkout status %q", locked.Status))
			}

			now := uc.clock.Now().UTC()
			if locked.IsExpiredAt(now) {
				return checkoutSessionExpiredError()
			}

			if locked.CreditKind == kind {
				session = locked
				return nil
			}

			if locked.CreditKind != "" && locked.DecisionID != nil && !locked.DecisionID.IsZero() {
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

			if locked.CreditKind == "" {
				selected, setErr := productQ.SetCreditKindBeforeDecision(ctx, locked.ID, kind, now)
				if setErr != nil {
					return setErr
				}

				session = selected
				changed = true
				return nil
			}

			selected, selectErr := productQ.SelectCreditKindBeforeDecision(ctx, locked.ID, kind, now)
			if selectErr != nil {
				return selectErr
			}

			session = selected
			changed = true
			return nil
		})
	})
	return session, changed, err
}
