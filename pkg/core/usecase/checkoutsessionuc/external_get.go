package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// GetExternal returns the read-only hosted page projection of an external
// checkout: merchant display name, amount, evaluation generation, the prepared
// offer (when linked), and the masked phone the verification code goes to.
//
// It is strictly read-only. It never calls Identity, creates a Decision, binds
// a customer, increments evaluation_generation, sends OTP, creates a Contract,
// creates a PaymentAttempt, or calls Financial Core. Open sessions past
// expires_at are projected as expired without writing state.
//
// The offer must already have been prepared during Partner Create. Offer
// economics are purchase-specific and privacy-safe: available_credit_minor,
// customer_reference, customer ids, and reason codes never appear.
func (uc *UseCase) GetExternal(ctx context.Context, input GetExternalInput) (GetExternalOutput, error) {
	const op = "checkoutsessionuc.UseCase.GetExternal"

	if err := input.validate(); err != nil {
		return GetExternalOutput{}, err
	}

	var (
		output  GetExternalOutput
		session model.CheckoutSession
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			sessionQ := uc.checkoutSession.Conn(conn)
			merchantQ := uc.merchantProfile.Conn(conn)

			found, err := sessionQ.GetByPublicToken(ctx, input.Token)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(err)
				}

				return err
			}

			if found.Origin != model.ExternalPartnerCheckoutSessionOrigin {
				return checkoutSessionNotFoundError(
					fmt.Errorf("%s: checkout session origin is %q", op, found.Origin),
				)
			}

			merchant, err := merchantQ.GetByID(ctx, found.MerchantProfileID)
			if err != nil {
				return err
			}

			phone, err := uc.decryptCustomerPhone(found)
			if err != nil {
				return err
			}

			status := found.Status
			if status == model.OpenCheckoutSessionStatus && found.ContractID == nil &&
				found.IsExpiredAt(uc.clock.Now().UTC()) {
				status = model.ExpiredCheckoutSessionStatus
			}

			session = found
			output = GetExternalOutput{
				Token:                found.PublicToken,
				Status:               status,
				MerchantDisplayName:  merchant.DisplayName,
				AmountMinor:          found.RequestedAmountMinor,
				Currency:             found.Currency,
				EvaluationGeneration: found.EvaluationGeneration,
				Verification: ExternalVerification{
					Type:        VerificationTypeSMSOTP,
					Destination: MaskCustomerPhone(phone),
				},
				ExpiresAt: found.ExpiresAt,
			}

			return nil
		},
	)
	if err != nil {
		return GetExternalOutput{}, err
	}

	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return output, nil
	}

	offer, projectErr := uc.projectPersistedOffer(ctx, session)
	if projectErr != nil {
		return GetExternalOutput{}, projectErr
	}

	// Hosted External never exposes account-level available credit.
	offer.AvailableCreditMinor = 0
	output.Offer = &offer

	return output, nil
}
