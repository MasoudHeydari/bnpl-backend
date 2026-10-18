package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type assignmentPrepareState struct {
	profile           model.CustomerProfile
	assessmentInput   InitialLimitAssessmentInput
	earlyOutput       *AssignInitialLimitOutput
	blockedReasonCode string
	selectedPolicy    initialcreditlimit.Policy
}

// AssignInitialLimit assigns the default initial credit limit for an eligible customer profile.
//
// The governed Decision remains the authority for why a limit exists. CreditAccount
// stores the single current operational state and credit_transactions keeps the
// immutable capacity-change history. The external assessment is deliberately done
// outside the database transaction.
func (uc *UseCase) AssignInitialLimit(ctx context.Context, input AssignInitialLimitInput) (AssignInitialLimitOutput, error) {
	const op = "customercredituc.UseCase.AssignInitialLimit"

	if input.ExternalCustomerID.IsZero() {
		return AssignInitialLimitOutput{}, externalCustomerIDRequiredError()
	}

	now := uc.clock.Now().UTC()
	prepare, err := uc.prepareInitialLimitAssignment(ctx, input.ExternalCustomerID, now)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	if prepare.earlyOutput != nil {
		output := *prepare.earlyOutput
		output.Result = AssignInitialLimitResultAlreadyAssigned
		return output, nil
	}

	var assessment InitialLimitAssessmentResult
	if prepare.blockedReasonCode != "" {
		assessment = uc.deniedProfileAssessment(prepare.blockedReasonCode, prepare.selectedPolicy)
	} else {
		assessment, err = uc.assessInitialLimitWithPolicy(ctx, prepare.assessmentInput, prepare.selectedPolicy)
		if err != nil {
			return AssignInitialLimitOutput{}, err
		}
	}

	assessment, err = normalizeAndValidateInitialLimitAssessment(assessment, now)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	if assessment.Currency != prepare.selectedPolicy.Terms().Currency {
		return AssignInitialLimitOutput{}, creditAssessmentUnavailableError(
			fmt.Errorf("%s: credit assessment currency does not match configured policy currency", op),
		)
	}

	if output, resolved, resolveErr := uc.tryResolveIdempotentAssignment(ctx, prepare.profile.ID, assessment); resolved {
		if resolveErr != nil {
			return AssignInitialLimitOutput{}, resolveErr
		}

		output.Result = resultFromResolvedOutput(output)
		return output, nil
	}

	return uc.persistInitialLimitAssignment(ctx, prepare.profile.ID, prepare.assessmentInput, assessment, now)
}

func (uc *UseCase) prepareInitialLimitAssignment(
	ctx context.Context,
	externalCustomerID model.ExternalCustomerID,
	now time.Time,
) (assignmentPrepareState, error) {
	var state assignmentPrepareState
	const op = "customercredituc.UseCase.prepareInitialLimitAssignment"

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			profileQ := uc.customerProfile.Tx(tx)
			accountQ := uc.creditAccount.Tx(tx)

			profile, err := uc.ensureCustomerProfileForInitialCredit(ctx, profileQ, externalCustomerID, now)
			if err != nil {
				return err
			}

			state.profile = profile

			policyAssignmentQ := uc.policyAssignment.Tx(tx)
			selectedPolicy, err := uc.resolveInitialLimitPolicy(ctx, policyAssignmentQ, externalCustomerID, now)
			if err != nil {
				return err
			}

			state.selectedPolicy = selectedPolicy

			if reasonCode, blocked := blockedCustomerProfileStatus(profile); blocked {
				state.blockedReasonCode = reasonCode
				return nil
			}

			currency := selectedPolicy.Terms().Currency
			if !currency.IsValid() {
				return cerr.InternalError(fmt.Errorf("%s: initial limit assessor currency is invalid", op))
			}

			if err = accountQ.ExpireDueByCustomerIDAndCurrency(ctx, profile.ID, currency, now); err != nil {
				return err
			}

			account, err := accountQ.LockByCustomerIDAndCurrency(ctx, profile.ID, currency)
			if err == nil && account.IsUsableAt(now) {
				decisionQ := uc.decision.Tx(tx)
				output, loadErr := uc.outputFromExistingCreditAccount(ctx, decisionQ, account)
				if loadErr != nil {
					return loadErr
				}

				state.earlyOutput = &output
				return nil
			}

			if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return err
			}

			state.assessmentInput = buildInitialLimitAssessmentInput(profile, now)
			return nil
		})
	})
	if err != nil {
		return assignmentPrepareState{}, err
	}

	return state, nil
}

func (uc *UseCase) tryResolveIdempotentAssignment(
	ctx context.Context,
	customerID model.CustomerID,
	assessment InitialLimitAssessmentResult,
) (AssignInitialLimitOutput, bool, error) {
	var output AssignInitialLimitOutput
	var resolved bool

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			accountQ := uc.creditAccount.Tx(tx)
			cycle, err := assignmentCycleFor(ctx, accountQ, customerID, assessment)
			if err != nil {
				return err
			}

			key := initialLimitIdempotencyKey(customerID, assessment, cycle)
			fingerprint, err := uc.initialLimitFingerprint(customerID, assessment, cycle)
			if err != nil {
				return err
			}

			decisionQ := uc.decision.Tx(tx)
			var found bool
			output, found, err = uc.resolveIdempotentAssignment(ctx, decisionQ, accountQ, key, fingerprint)
			resolved = found
			return err
		})
	})
	return output, resolved, err
}

func (uc *UseCase) persistInitialLimitAssignment(
	ctx context.Context,
	customerID model.CustomerID,
	expectedAssessmentInput InitialLimitAssessmentInput,
	assessment InitialLimitAssessmentResult,
	now time.Time,
) (AssignInitialLimitOutput, error) {
	var output AssignInitialLimitOutput
	const op = "customercredituc.UseCase.persistInitialLimitAssignment"

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			profileQ := uc.customerProfile.Tx(tx)
			accountQ := uc.creditAccount.Tx(tx)
			decisionQ := uc.decision.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			profile, err := profileQ.LockByID(ctx, customerID)
			if err != nil {
				return err
			}

			if reasonCode, blocked := blockedCustomerProfileStatus(profile); blocked {
				denied := uc.deniedProfileAssessmentFromAssessment(reasonCode, assessment)
				persisted, err := uc.persistDeniedAssignmentInTx(ctx, decisionQ, profile, profile.Jurisdiction, denied, now)
				if err != nil {
					return err
				}

				err = uc.insertAssignInitialLimitAuditOutbox(ctx, outboxQ, profile, persisted, denied, now)
				if err != nil {
					return err
				}

				output = withAssignInitialLimitResult(persisted, AssignInitialLimitResultDenied)
				return nil
			}

			if current := buildInitialLimitAssessmentInput(profile, now); !sameAssessmentInput(current, expectedAssessmentInput) {
				return customerProfileChangedConflictError(fmt.Errorf("%s: customer profile changed during initial limit assignment", op))
			}

			if err = accountQ.ExpireDueByCustomerIDAndCurrency(ctx, profile.ID, assessment.Currency, now); err != nil {
				return err
			}

			account, accountErr := accountQ.LockByCustomerIDAndCurrency(ctx, profile.ID, assessment.Currency)
			if accountErr == nil && account.IsUsableAt(now) {
				existing, err := uc.outputFromExistingCreditAccount(ctx, decisionQ, account)
				if err != nil {
					return err
				}

				output = withAssignInitialLimitResult(existing, AssignInitialLimitResultAlreadyAssigned)
				return nil
			}

			if accountErr != nil && !cerr.IsCode(accountErr, cerr.CodeResourceNotFound) {
				return accountErr
			}

			cycle, err := assignmentCycleFor(ctx, accountQ, profile.ID, assessment)
			if err != nil {
				return err
			}

			key := initialLimitIdempotencyKey(profile.ID, assessment, cycle)
			fingerprint, err := uc.initialLimitFingerprint(profile.ID, assessment, cycle)
			if err != nil {
				return err
			}

			if resolvedOutput, found, err := uc.resolveIdempotentAssignment(ctx, decisionQ, accountQ, key, fingerprint); found {
				if err != nil {
					return err
				}

				output = withAssignInitialLimitResult(resolvedOutput, resultFromResolvedOutput(resolvedOutput))
				return nil
			}

			if assessment.Outcome == model.DeniedDecisionOutcome {
				persisted, err := uc.persistDeniedAssignmentInTx(ctx, decisionQ, profile, profile.Jurisdiction, assessment, now)
				if err != nil {
					return err
				}

				if err := uc.insertAssignInitialLimitAuditOutbox(ctx, outboxQ, profile, persisted, assessment, now); err != nil {
					return err
				}

				output = withAssignInitialLimitResult(persisted, AssignInitialLimitResultDenied)
				return nil
			}

			persisted, err := uc.persistApprovedAssignmentInTx(
				ctx, decisionQ, accountQ, profile, account, accountErr == nil, assessment, key, fingerprint, now,
			)
			if err != nil {
				return err
			}

			if err := uc.insertAssignInitialLimitAuditOutbox(ctx, outboxQ, profile, persisted, assessment, now); err != nil {
				return err
			}

			output = withAssignInitialLimitResult(persisted, AssignInitialLimitResultCreated)
			return nil
		})
	})
	if err != nil {
		if isPlausibleIdempotencyCreateConflict(err) {
			resolved, resolveErr := uc.resolveAssignmentAfterConflict(ctx, customerID, assessment, now)
			if resolveErr == nil {
				return withAssignInitialLimitResult(resolved, resultFromResolvedOutput(resolved)), nil
			}
		}

		return AssignInitialLimitOutput{}, err
	}

	return output, nil
}

func withAssignInitialLimitResult(output AssignInitialLimitOutput, result AssignInitialLimitResult) AssignInitialLimitOutput {
	output.Result = result
	return output
}

func blockedCustomerProfileStatus(profile model.CustomerProfile) (string, bool) {
	if profile.IsActive() {
		return "", false
	}

	return model.CustomerProfileNotActiveDecisionReason, true
}

func (uc *UseCase) ensureCustomerProfileForInitialCredit(
	ctx context.Context,
	profileQ repo.CustomerProfileTxQueryer,
	externalCustomerID model.ExternalCustomerID,
	now time.Time,
) (model.CustomerProfile, error) {
	profile, _, err := profileQ.EnsureExists(ctx, externalCustomerID, model.IRJurisdiction, model.ActiveCustomerStatus, now, now)
	return profile, err
}

func buildInitialLimitAssessmentInput(profile model.CustomerProfile, now time.Time) InitialLimitAssessmentInput {
	return InitialLimitAssessmentInput{
		CustomerID: profile.ID, ExternalCustomerID: profile.ExternalCustomerID,
		Jurisdiction: profile.Jurisdiction, RequestedAt: now,
	}
}

func sameAssessmentInput(current, expected InitialLimitAssessmentInput) bool {
	return current.CustomerID == expected.CustomerID &&
		current.ExternalCustomerID == expected.ExternalCustomerID &&
		current.Jurisdiction == expected.Jurisdiction
}

func (uc *UseCase) deniedProfileAssessment(reasonCode string, selected initialcreditlimit.Policy) InitialLimitAssessmentResult {
	terms := selected.Terms()
	return InitialLimitAssessmentResult{
		Outcome: model.DeniedDecisionOutcome, Currency: terms.Currency,
		PolicyID: selected.Ref().ID, PolicyVersion: selected.Ref().Version,
		ReasonCodes: []string{reasonCode},
	}
}

func (uc *UseCase) deniedProfileAssessmentFromAssessment(reasonCode string, prior InitialLimitAssessmentResult) InitialLimitAssessmentResult {
	return InitialLimitAssessmentResult{
		Outcome: model.DeniedDecisionOutcome, Currency: prior.Currency,
		PolicyID: prior.PolicyID, PolicyVersion: prior.PolicyVersion,
		ReasonCodes: []string{reasonCode},
	}
}

func (uc *UseCase) persistDeniedAssignmentInTx(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	profile model.CustomerProfile,
	jurisdiction model.Jurisdiction,
	assessment InitialLimitAssessmentResult,
	now time.Time,
) (AssignInitialLimitOutput, error) {
	key := initialLimitIdempotencyKey(profile.ID, assessment, 0)
	fingerprint, err := uc.initialLimitFingerprint(profile.ID, assessment, 0)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	decision, err := decisionQ.Create(
		ctx, model.CreditLimitDecisionType, model.CustomerProfileDecisionTargetType, profile.ID.UUID(),
		model.DeniedDecisionOutcome, assessment.ReasonCodes, assessment.PolicyID, assessment.PolicyVersion,
		jurisdiction, model.SystemDecisionActorType, nil, nil, nil, nil, now, now, key, fingerprint,
	)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	return AssignInitialLimitOutput{Decision: decision}, nil
}

func (uc *UseCase) persistApprovedAssignmentInTx(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	accountQ repo.CreditAccountTxQueryer,
	profile model.CustomerProfile,
	existingAccount model.CreditAccount,
	hasExistingAccount bool,
	assessment InitialLimitAssessmentResult,
	idempotencyKey string,
	fingerprint string,
	now time.Time,
) (AssignInitialLimitOutput, error) {
	decision, err := decisionQ.Create(
		ctx,
		model.CreditLimitDecisionType,
		model.CustomerProfileDecisionTargetType,
		profile.ID.UUID(),
		model.LimitSetDecisionOutcome,
		assessment.ReasonCodes,
		assessment.PolicyID,
		assessment.PolicyVersion,
		profile.Jurisdiction,
		model.SystemDecisionActorType,
		nil,
		nil,
		nil,
		nil,
		now,
		now,
		idempotencyKey,
		fingerprint,
	)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	expiresAt := now.Add(assessment.ValidFor)
	var account model.CreditAccount
	if hasExistingAccount {
		account, err = accountQ.SetLimit(ctx, existingAccount.ID, assessment.AmountMinor, decision.ID, now, &expiresAt, now)
	} else {
		account, err = accountQ.CreateWithLimit(ctx, profile.ID, assessment.Currency, assessment.AmountMinor, decision.ID, now, &expiresAt, now, now)
	}

	if err != nil {
		return AssignInitialLimitOutput{}, err
	}

	txType, txAmount := limitChangeJournal(existingAccount, hasExistingAccount, assessment.AmountMinor)
	if _, err = accountQ.AppendTransaction(
		ctx,
		account.ID,
		txType,
		txAmount,
		assessment.Currency,
		model.CreditAssessmentCreditTransactionReason,
		model.DecisionCreditTransactionReferenceType,
		decision.ID.UUID(),
		now,
		now,
	); err != nil {
		return AssignInitialLimitOutput{}, err
	}

	return AssignInitialLimitOutput{Decision: decision, CreditLimit: &account}, nil
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
