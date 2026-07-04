package decisionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// RevalidateCheckoutOfferInput identifies a persisted checkout Decision whose
// offer must be checked against current mutable customer/merchant/credit state.
type RevalidateCheckoutOfferInput struct {
	// DecisionID is the Decision currently linked to the CheckoutSession.
	DecisionID model.DecisionID
}

// RevalidateCheckoutOfferOutput reports whether the persisted offer can still
// be honored without changing customer-facing material terms.
//
// StillHonorable means Confirm may proceed with the existing Decision. When
// false, CheckoutSession must bump evaluation_generation and Create a fresh
// Decision through the normal pipeline (current policy assignment).
type RevalidateCheckoutOfferOutput struct {
	// StillHonorable is true when current state still supports the persisted
	// financed/upfront terms under the Decision's policy snapshot.
	StillHonorable bool
}

func (i RevalidateCheckoutOfferInput) validate() error {
	const op = "decisionuc.RevalidateCheckoutOfferInput.validate"

	if i.DecisionID.IsZero() {
		return cerr.InvalidArgument(fmt.Errorf("%s: decision_id is required", op))
	}

	return nil
}

// RevalidateCheckoutOffer re-runs the Decision eligibility/financing pipeline
// against current mutable state using the persisted Decision's PolicyID /
// PolicyVersion snapshot.
//
// It never persists a Decision, never consults the active policy assignment for
// the existing offer, and never treats a raw available-credit change alone as a
// material change. A runtime policy-assignment change therefore cannot invalidate
// an already-valid persisted Decision by itself.
func (uc *UseCase) RevalidateCheckoutOffer(
	ctx context.Context,
	input RevalidateCheckoutOfferInput,
) (RevalidateCheckoutOfferOutput, error) {
	const op = "decisionuc.UseCase.RevalidateCheckoutOffer"

	if err := input.validate(); err != nil {
		return RevalidateCheckoutOfferOutput{}, err
	}

	now := uc.clock.Now().UTC()
	var output RevalidateCheckoutOfferOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		decisionQ := uc.decision.Conn(conn)
		allocationQ := uc.checkoutDecisionAllocation.Conn(conn)
		customerProfileQ := uc.customerProfile.Conn(conn)
		creditAccountQ := uc.creditAccount.Conn(conn)
		merchantProfileQ := uc.merchantProfile.Conn(conn)
		merchantBranchProfileQ := uc.merchantBranchProfile.Conn(conn)

		decision, err := decisionQ.GetByID(ctx, input.DecisionID)
		if err != nil {
			return fmt.Errorf("%s: load decision: %w", op, err)
		}

		if decision.ExpiresAt != nil && !decision.ExpiresAt.After(now) {
			output.StillHonorable = false

			return nil
		}

		if !outcomeCreatesInstallmentOffer(decision.Outcome) {
			output.StillHonorable = false

			return nil
		}

		allocation, err := allocationQ.GetByDecisionID(ctx, decision.ID)
		if err != nil {
			return fmt.Errorf("%s: load allocation: %w", op, err)
		}

		selectedPolicy, err := uc.resolveCheckoutPolicyRef(policy.Ref{
			ID:      decision.PolicyID,
			Version: decision.PolicyVersion,
		})
		if err != nil {
			return err
		}

		customerProfile, err := customerProfileQ.GetByID(ctx, allocation.CustomerID)
		if err != nil {
			return fmt.Errorf("%s: load customer profile: %w", op, err)
		}

		merchantProfile, err := merchantProfileQ.GetByID(ctx, allocation.MerchantProfileID)
		if err != nil {
			return fmt.Errorf("%s: load merchant profile: %w", op, err)
		}

		var branchProfile *model.MerchantBranchProfile
		if allocation.MerchantBranchProfileID != nil && !allocation.MerchantBranchProfileID.IsZero() {
			branch, branchErr := merchantBranchProfileQ.GetByID(ctx, *allocation.MerchantBranchProfileID)
			if branchErr != nil {
				return fmt.Errorf("%s: load merchant branch profile: %w", op, branchErr)
			}

			branchProfile = &branch
		}

		creditAccount, hasCreditAccount, err := loadCurrentCreditAccount(
			ctx,
			creditAccountQ,
			customerProfile.ID,
			allocation.Currency,
			now,
		)
		if err != nil {
			return fmt.Errorf("%s: load credit account: %w", op, err)
		}

		_, financing := evaluateCheckoutTerms(
			selectedPolicy,
			now,
			allocation.RequestedAmountMinor,
			allocation.Currency,
			customerProfile,
			merchantProfile,
			branchProfile,
			creditAccount,
			hasCreditAccount,
		)

		output.StillHonorable = materialOfferTermsMatch(allocation, financing)

		return nil
	})
	if err != nil {
		return RevalidateCheckoutOfferOutput{}, err
	}

	return output, nil
}
