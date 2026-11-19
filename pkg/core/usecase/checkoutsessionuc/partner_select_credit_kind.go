package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// SelectCreditKindInput is the Partner continuation that records or replaces
// the customer-chosen credit kind after the merchant-site selection UI.
type SelectCreditKindInput struct {
	// Token is the public checkout session token returned by CreatePartner.
	Token model.CheckoutSessionToken
	// ExternalMerchantID must match the terminal-authenticated merchant.
	ExternalMerchantID model.ExternalMerchantID
	// CreditKind is the customer-selected kind (pay_in_installments or pay_next_month).
	CreditKind model.CreditKind
	// ExpectedEvaluationGeneration is the optimistic-concurrency token from
	// Partner Create or the previous SelectCreditKind response. Mandatory for
	// every selection mutation, including the first selection.
	ExpectedEvaluationGeneration int
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

	if i.ExpectedEvaluationGeneration < 1 {
		return validationError(
			"REQUIRED",
			"expected_evaluation_generation",
			"error.expected_evaluation_generation_required",
		)
	}

	return nil
}

// SelectCreditKind persists or replaces the merchant-communicated credit-kind
// selection. Hosted START must not override this value.
//
// Same-kind PUT is idempotent: generation is not advanced and existing
// Decision / OTP context is preserved. A different kind on an still-editable
// open session clears the current Decision linkage and advances
// evaluation_generation exactly once. Prior Decisions remain immutable history.
//
// After a successful mutation (or same-kind recovery), on_create sessions
// prepare the Offer outside the selection transaction.
func (uc *UseCase) SelectCreditKind(
	ctx context.Context,
	input SelectCreditKindInput,
) (SelectCreditKindOutput, error) {
	if err := input.validate(); err != nil {
		return SelectCreditKindOutput{}, err
	}

	session, err := uc.selectPartnerCreditKind(
		ctx,
		input.Token,
		input.ExternalMerchantID,
		input.CreditKind,
		input.ExpectedEvaluationGeneration,
	)
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
	expectedGeneration int,
) (session model.CheckoutSession, err error) {
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

			if locked.Status != model.OpenCheckoutSessionStatus {
				return checkoutSessionNotContinuableError(fmt.Errorf(
					"%s: checkout status %q", op, locked.Status,
				))
			}

			if locked.ContractID != nil && !locked.ContractID.IsZero() {
				return checkoutSessionNotContinuableError(fmt.Errorf(
					"%s: checkout already linked to a contract", op,
				))
			}

			if locked.ConfirmationAttemptID != nil && !locked.ConfirmationAttemptID.IsZero() {
				return checkoutConfirmationInProgressError(fmt.Errorf(
					"%s: confirmation owns the checkout", op,
				))
			}

			now := uc.clock.Now().UTC()
			if locked.IsExpiredAt(now) {
				return checkoutSessionExpiredError()
			}

			// Same-kind idempotent replay: return durable state without mutating
			// generation, Decision linkage, or OTP context. Stale expected
			// generations are still accepted when the kind already matches so a
			// delayed retry of a successful selection cannot fail after commit.
			if locked.CreditKind == kind {
				session = locked
				return nil
			}

			if locked.EvaluationGeneration != expectedGeneration {
				return checkoutCreditKindSelectionStaleError(fmt.Errorf(
					"%s: expected evaluation generation %d but session is at %d",
					op,
					expectedGeneration,
					locked.EvaluationGeneration,
				))
			}

			enabled, enableErr := merchantQ.IsCreditKindEnabled(ctx, locked.MerchantProfileID, kind)
			if enableErr != nil {
				return enableErr
			}

			if !enabled {
				return validationError("INVALID", "credit_kind", "error.credit_kind_not_enabled")
			}

			selected, replaceErr := checkoutSessionQ.ReplaceCreditKindAndInvalidateEvaluation(
				ctx,
				locked.ID,
				kind,
				expectedGeneration,
				locked.DecisionID,
				now,
			)
			if replaceErr != nil {
				return replaceErr
			}

			session = selected
			return nil
		})
	})

	return session, err
}
