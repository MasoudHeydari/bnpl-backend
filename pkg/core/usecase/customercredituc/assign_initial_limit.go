package customercredituc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// AssignInitialLimit submits or resumes a durable request for an independent credit product.
//
// The governed Decision remains the authority for why a limit exists. CreditAccount
// stores the single current operational state and credit_transactions keeps the
// immutable capacity-change history. Identity resolution is performed outside
// database transactions; fixed-limit allocation does not call the Credit Bureau.
func (uc *UseCase) AssignInitialLimit(ctx context.Context, input AssignInitialLimitInput) (AssignInitialLimitOutput, error) {
	return uc.submitCreditRequest(ctx, input)
}

// observeAssignment reports a committed or replayed assignment outcome.
// Pending credit-request states have no Decision yet and must not emit
// assignment observers with a zero DecisionID.
func (uc *UseCase) observeAssignment(
	ctx context.Context,
	customerID model.CustomerID,
	output AssignInitialLimitOutput,
) {
	if output.Decision.ID.IsZero() {
		return
	}

	switch output.Result {
	case AssignInitialLimitResultCreated:
		uc.observer.LimitAssigned(ctx, customerID, output.Decision.ID)

	case AssignInitialLimitResultAlreadyAssigned:
		uc.observer.LimitAlreadyAssigned(ctx, customerID, output.Decision.ID)

	case AssignInitialLimitResultDenied:
		reasonCode := ""
		if output.CreditRequest != nil {
			reasonCode = output.CreditRequest.ReasonCode
		}

		if len(output.Decision.ReasonCodes) > 0 {
			reasonCode = output.Decision.ReasonCodes[0]
		}

		uc.observer.LimitAssignmentDenied(ctx, customerID, output.Decision.ID, reasonCode)
	}
}

func (uc *UseCase) ensureCustomerProfileForInitialCredit(
	ctx context.Context,
	profileQ repo.CustomerProfileTxQueryer,
	externalSubjectID model.ExternalSubjectID,
	externalCustomerID model.ExternalCustomerID,
	nationalCode string,
	now time.Time,
) (model.CustomerProfile, error) {
	profile, _, err := profileQ.EnsureExists(
		ctx,
		externalSubjectID,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
		nationalCode,
	)
	return profile, err
}

// limitChangeJournal selects the CreditTransaction type/amount for a governed
// limit mutation. First creation and same-amount refresh use LIMIT_SET with the
// absolute limit. Amount changes journal the positive delta as INCREASED/DECREASED
// without mutating exposure.
func limitChangeJournal(
	existing model.CreditAccount,
	hasExistingAccount bool,
	newAmountMinor uint64,
) (model.CreditTransactionType, uint64) {
	if !hasExistingAccount {
		return model.LimitSetCreditTransactionType, newAmountMinor
	}

	previous := existing.LimitAmountMinor
	switch {
	case newAmountMinor > previous:
		return model.LimitIncreasedCreditTransactionType, newAmountMinor - previous

	case newAmountMinor < previous:
		return model.LimitDecreasedCreditTransactionType, previous - newAmountMinor

	default:
		return model.LimitSetCreditTransactionType, newAmountMinor
	}
}
