package customercredituc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// assignMonthlyCreditLimit is a separately governed capacity assignment.
// Existing installment capacity/exposure must not be copied or modified.
// Credit assessment takes place outside transactions, and the customer row
// serializes concurrent assignments to the same monthly credit account.
func (uc *UseCase) assignMonthlyCreditLimit(
	ctx context.Context, input AssignInitialLimitInput,
) (AssignInitialLimitOutput, error) {
	const op = "customercredituc.UseCase.assignMonthlyCreditLimit"

	externalCustomerID, err := uc.resolveActiveExternalCustomerID(ctx, input.MobileE164)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}
	now := uc.clock.Now().UTC()
	externalSubjectID := input.SubjectID.AsExternalSubjectID()

	var profile model.CustomerProfile
	var prior *AssignInitialLimitOutput
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			var getErr error
			profile, getErr = uc.ensureCustomerProfileForInitialCredit(
				ctx, uc.customerProfile.Tx(tx), externalSubjectID, externalCustomerID, now,
			)
			if getErr != nil { return getErr }
			if !profile.IsActive() { return cerr.Conflict(fmt.Errorf("%s: customer profile is not active", op)) }

			q, ok := uc.creditAccount.Tx(tx).(repo.CreditAccountProductTxQueryer)
			if !ok { return cerr.InternalError(fmt.Errorf("%s: product credit account repository unavailable", op)) }
			currency := model.IRRCurrency
			if err := q.ExpireDueByCustomerIDCurrencyAndKind(ctx, profile.ID, currency, model.PayNextMonthCreditKind, now); err != nil { return err }
			account, getErr := q.LockByCustomerIDCurrencyAndKind(ctx, profile.ID, currency, model.PayNextMonthCreditKind)
			if getErr == nil && account.IsUsableAt(now) {
				existing, outErr := uc.outputFromExistingCreditAccount(ctx, uc.decision.Tx(tx), account)
				if outErr != nil { return outErr }
				existing.Result = AssignInitialLimitResultAlreadyAssigned
				prior = &existing
				return nil
			}
			if getErr != nil && !cerr.IsCode(getErr, cerr.CodeResourceNotFound) { return getErr }
			return nil
		})
	})
	if err != nil { return AssignInitialLimitOutput{}, err }
	if prior != nil { return *prior, nil }

	// Resolve assigned initial-credit assessment policy for this customer.
	// The signal fetch is deliberately outside the database transaction.
	assessed, err := uc.AssessInitialLimit(ctx, buildInitialLimitAssessmentInput(profile, now))
	if err != nil { return AssignInitialLimitOutput{}, err }
	assessed, err = normalizeAndValidateInitialLimitAssessment(assessed, now)
	if err != nil { return AssignInitialLimitOutput{}, err }
	if assessed.Currency != model.IRRCurrency {
		return AssignInitialLimitOutput{}, cerr.InternalError(fmt.Errorf("%s: monthly credit requires IRR", op))
	}

	var output AssignInitialLimitOutput
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			currentProfile, err := uc.customerProfile.Tx(tx).LockByID(ctx, profile.ID)
			if err != nil { return err }
			if !currentProfile.IsActive() || currentProfile.ExternalCustomerID != externalCustomerID {
				return cerr.Conflict(fmt.Errorf("%s: customer changed during assessment", op))
			}
			q, ok := uc.creditAccount.Tx(tx).(repo.CreditAccountProductTxQueryer)
			if !ok { return cerr.InternalError(fmt.Errorf("%s: product credit repository unavailable", op)) }
			if err := q.ExpireDueByCustomerIDCurrencyAndKind(ctx, profile.ID, assessed.Currency, model.PayNextMonthCreditKind, now); err != nil {
				return err
			}
			existing, getErr := q.LockByCustomerIDCurrencyAndKind(ctx, profile.ID, assessed.Currency, model.PayNextMonthCreditKind)
			if getErr == nil && existing.IsUsableAt(now) {
				output, err = uc.outputFromExistingCreditAccount(ctx, uc.decision.Tx(tx), existing)
				output.Result = AssignInitialLimitResultAlreadyAssigned
				return err
			}
			if getErr != nil && !cerr.IsCode(getErr, cerr.CodeResourceNotFound) { return getErr }

			// The account version fences the next assessment cycle for the
			// monthly product; the customer lock serializes writers.
			cycle := int64(0)
			if getErr == nil { cycle = existing.Version }
			key := fmt.Sprintf("initial-monthly-credit:%s:%d", profile.ID, cycle)
			fingerprint, err := uc.fingerprintHasher.Hash(
				fmt.Sprintf("initial-monthly-credit:v1:%s:%d:%s:%s:%d", profile.ID, cycle, assessed.PolicyID, assessed.PolicyVersion, assessed.AmountMinor),
			)
			if err != nil { return err }
			if err := fingerprint.Validate(); err != nil { return err }
			outcome := assessed.Outcome
			if outcome != model.LimitSetDecisionOutcome && outcome != model.DeniedDecisionOutcome {
				return cerr.InternalError(fmt.Errorf("%s: unexpected assessment outcome", op))
			}
			decisionQ := uc.decision.Tx(tx)
			decision, err := decisionQ.Create(
				ctx, model.CreditLimitDecisionType, model.CustomerProfileDecisionTargetType, profile.ID.UUID(),
				outcome, assessed.ReasonCodes, assessed.PolicyID, assessed.PolicyVersion,
				profile.Jurisdiction, model.SystemDecisionActorType, nil, nil, nil, nil,
				now, now, key, fingerprint.String(),
			)
			if err != nil { return err }
			output = AssignInitialLimitOutput{Decision:decision, Result:AssignInitialLimitResultDenied}
			if outcome == model.DeniedDecisionOutcome { return nil }

			expires := now.Add(assessed.ValidFor)
			var account model.CreditAccount
			if getErr == nil {
				account, err = q.SetLimit(ctx, existing.ID, assessed.AmountMinor, decision.ID, now, &expires, now)
			} else {
				account, err = q.CreateWithLimitForKind(ctx, profile.ID, assessed.Currency,
					model.PayNextMonthCreditKind, assessed.AmountMinor, decision.ID, now, &expires, now, now)
			}
			if err != nil { return err }
			journalType, journalAmount := limitChangeJournal(existing, getErr == nil, assessed.AmountMinor)
			if journalAmount == 0 {
				// Equal-amount reassessment must not insert a zero-value
				// journal event (the journal requires a positive amount).
				journalType = model.LimitSetCreditTransactionType
				journalAmount = assessed.AmountMinor
			}
			_, err = q.AppendTransaction(ctx, account.ID, journalType, journalAmount,
				assessed.Currency, model.CreditAssessmentCreditTransactionReason,
				model.DecisionCreditTransactionReferenceType, decision.ID.UUID(), now, now)
			if err != nil { return err }
			output.CreditLimit = &account
			output.Result = AssignInitialLimitResultCreated
			return nil
		})
	})
	if err != nil { return AssignInitialLimitOutput{}, err }
	return output, nil
}

