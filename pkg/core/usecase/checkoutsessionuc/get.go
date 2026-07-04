package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// GetInput reads a hosted checkout session for the authenticated customer.
type GetInput struct {
	CheckoutSessionID model.CheckoutSessionID
	IdentitySubjectID model.IdentitySubjectID
}

func (in GetInput) validate() error {
	if in.CheckoutSessionID.IsZero() {
		return checkoutSessionIDRequiredError()
	}

	if in.IdentitySubjectID.IsZero() {
		return identitySubjectIDRequiredError()
	}

	return nil
}

// GetOutput is the read-only hosted checkout projection.
type GetOutput struct {
	Session model.CheckoutSession
	Offer   *Offer
}

// Get returns the current checkout session and any already-prepared offer for
// the authenticated customer. It never binds a customer, creates a Decision,
// bumps evaluation_generation, or emits lifecycle events.
func (uc *UseCase) Get(ctx context.Context, input GetInput) (GetOutput, error) {
	const op = "checkoutsessionuc.UseCase.Get"

	if err := input.validate(); err != nil {
		return GetOutput{}, err
	}

	var (
		session model.CheckoutSession
		offer   *Offer
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			sessionQ := uc.checkoutSession.Conn(conn)
			customerProfileQ := uc.customerProfile.Conn(conn)
			decisionQ := uc.decisionRepo.Conn(conn)
			allocationQ := uc.checkoutDecisionAllocation.Conn(conn)
			installmentQ := uc.installment.Conn(conn)
			installmentChargeQ := uc.installmentCharge.Conn(conn)
			merchantQ := uc.merchantProfile.Conn(conn)

			found, findErr := sessionQ.GetByID(ctx, input.CheckoutSessionID)
			if findErr != nil {
				if cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(findErr)
				}

				return findErr
			}

			profile, profileErr := customerProfileQ.GetByIdentitySubjectID(ctx, input.IdentitySubjectID)
			if profileErr != nil {
				return profileErr
			}

			if bindingErr := assertBoundCustomer(found, profile.ID); bindingErr != nil {
				return bindingErr
			}

			// Touch merchant profile so unauthorized cross-tenant ids fail closed
			// the same way as other merchant-facing reads.
			if _, merchantErr := merchantQ.GetByID(ctx, found.MerchantProfileID); merchantErr != nil {
				return merchantErr
			}

			session = found

			if found.DecisionID == nil || found.DecisionID.IsZero() {
				return nil
			}

			decision, decisionErr := decisionQ.GetByID(ctx, *found.DecisionID)
			if decisionErr != nil {
				if cerr.IsCode(decisionErr, cerr.CodeResourceNotFound) {
					return nil
				}

				return decisionErr
			}

			outcome := offerOutcome(decision.Outcome)
			if outcome == DeniedOfferOutcome {
				denied := Offer{Outcome: DeniedOfferOutcome}
				offer = &denied

				return nil
			}

			allocation, allocationErr := allocationQ.GetByDecisionID(ctx, *found.DecisionID)
			if allocationErr != nil {
				if cerr.IsCode(allocationErr, cerr.CodeResourceNotFound) {
					return nil
				}

				return allocationErr
			}

			installments, listErr := installmentQ.ListByDecisionID(ctx, *found.DecisionID)
			if listErr != nil {
				return listErr
			}

			installmentIDs := make([]model.InstallmentID, 0, len(installments))
			for _, row := range installments {
				installmentIDs = append(installmentIDs, row.ID)
			}

			charges, chargeErr := installmentChargeQ.ListByInstallmentIDs(ctx, installmentIDs)
			if chargeErr != nil {
				return chargeErr
			}

			selected, resolveErr := uc.checkoutPolicies.Resolve(policy.Ref{
				ID:      decision.PolicyID,
				Version: decision.PolicyVersion,
			})
			if resolveErr != nil {
				return fmt.Errorf("%s: resolve checkout policy: %w", op, resolveErr)
			}

			projected, projectErr := offerFromPersistedDecision(
				decision,
				found,
				allocation,
				installments,
				charges,
				selected,
			)
			if projectErr != nil {
				return fmt.Errorf("%s: project persisted offer: %w", op, projectErr)
			}

			offer = &projected

			return nil
		},
	)
	if err != nil {
		return GetOutput{}, err
	}

	return GetOutput{Session: session, Offer: offer}, nil
}

// offerFromPersistedDecision rebuilds the hosted offer projection from durable
// Decision/allocation/installment rows without inventing financed/upfront
// amounts from installment totals.
func offerFromPersistedDecision(
	decision model.Decision,
	session model.CheckoutSession,
	allocation model.CheckoutDecisionAllocation,
	installments []model.Installment,
	charges []model.InstallmentCharge,
	selected checkout.Policy,
) (Offer, error) {
	schedule, action, err := decisionuc.ProjectCheckoutPresentation(
		selected,
		allocation,
		installments,
		charges,
	)
	if err != nil {
		return Offer{}, err
	}

	expiresAt := decision.ExpiresAt
	if expiresAt == nil {
		expires := allocation.ExpiresAt
		expiresAt = &expires
	}

	offer := Offer{
		Outcome:                        offerOutcome(decision.Outcome),
		AvailableCreditMinor:           allocation.AvailableCreditMinor,
		RequestedAmountMinor:           session.RequestedAmountMinor,
		ApprovedFinancedAmountMinor:    allocation.ApprovedFinancedAmountMinor,
		RequiredDownPaymentAmountMinor: allocation.UpfrontAmountMinor,
		Currency:                       session.Currency,
		OfferExpiresAt:                 expiresAt,
		InstallmentCount:               allocation.InstallmentCount,
		PayableNowAmountMinor:          schedule.PayableNowAmountMinor,
		TotalPayableAmountMinor:        schedule.TotalPayableAmountMinor,
		CheckoutAction:                 &action,
		Installments:                   make([]OfferInstallment, 0, len(installments)),
	}

	for _, row := range installments {
		offer.Installments = append(offer.Installments, OfferInstallment{
			InstallmentOrder: row.InstallmentOrder,
			DueAt:            row.DueAt,
			TotalAmountMinor: row.TotalAmountMinor,
		})
	}

	return offer, nil
}
