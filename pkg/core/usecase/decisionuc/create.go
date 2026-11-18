package decisionuc

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout/businessrule"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreateInput carries checkout decision material inputs.
//
// ExternalSubjectID is the authenticated Identity Service subject ID
// (x-auth-subject-id / customer_profiles.external_subject_id). It is resolved via
// LockByExternalSubjectID to the BNPL customer profile primary key.
//
// PurchaseContext explicitly identifies whether checkout originated from a
// physical POS terminal or an online storefront. The use case resolves the
// corresponding selling point from BNPL-owned source mappings, then derives
// the parent merchant from branchProfile.ExternalMerchantID. Callers never supply a
// merchant identity or branch identity directly.
//
// Persisted allocation, installment, and exposure rows use the corresponding
// BNPL profile primary keys (customer_profiles.id / merchant_profiles.id /
// merchant_branch_profiles.id).
type CreateInput struct {
	// CreditKind chooses one credit product for the whole purchase. Omission
	// preserves the pay-in-four contract for legacy checkout clients.
	// Merchant code must not be allowed to change this after confirmation.
	CreditKind model.CreditKind

	// CheckoutKey is the checkout idempotency key.
	CheckoutKey string

	// ExternalSubjectID is the Identity Service subject mapped to customer_profiles.external_subject_id.
	ExternalSubjectID model.ExternalSubjectID
	// PurchaseContext identifies the authoritative merchant sales source for this checkout.
	PurchaseContext PurchaseContext

	// RequestedAmountMinor is the requested amount in minor currency units.
	RequestedAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
}

// CreateOutput returns the persisted checkout decision inquiry summary.
//
// An approved or partially approved (conditional) decision atomically creates
// and returns the persisted installment offer. Denied decisions omit
// InstallmentOffer.
type CreateOutput struct {
	// Result reports the use-case outcome classification.
	Result CreateResult

	// Decision is the persisted decision record.
	Decision model.Decision

	// PurchaseContext is the persisted sales-source snapshot used to resolve merchant context.
	PurchaseContext PurchaseContext

	// CheckoutAction tells the client how an approved offer is confirmed.
	CheckoutAction *CheckoutAction

	// RequestedAmountMinor is the requested amount in minor currency units.
	RequestedAmountMinor uint64
	// ApprovedFinancedAmountMinor is the approved financed amount in minor currency units.
	ApprovedFinancedAmountMinor uint64
	// UpfrontAmountMinor is the upfront amount in minor currency units.
	UpfrontAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// AvailableCreditMinor is the available credit in minor currency units at decision time.
	AvailableCreditMinor uint64
	// ExpiresAt is when the offer, session, or limit expires.
	ExpiresAt *time.Time

	// InstallmentOffer describes the installment offer when approval includes a schedule.
	InstallmentOffer *InstallmentOffer
}

// Create evaluates checkout eligibility at the current moment, persists a governed decision
// and allocation snapshot, and for approved outcomes atomically persists the installment offer.
// Approval does not reserve credit; capacity is re-checked before the payment_pending contract is created.
func (uc *UseCase) Create(ctx context.Context, input CreateInput) (CreateOutput, error) {
	const op = "decisionuc.UseCase.Create"

	checkoutKey, err := model.NormalizeCheckoutKey(input.CheckoutKey)
	if err != nil {
		return CreateOutput{}, invalidCheckoutKeyError(err)
	}

	input.CheckoutKey = checkoutKey

	if err := validateCreateInput(input); err != nil {
		return CreateOutput{}, err
	}

	uc.observer.EvaluationStarted(ctx, input.ExternalSubjectID)

	now := uc.clock.Now().UTC()
	var output CreateOutput
	var fingerprint string

	i := func(ctx context.Context, tx repo.Tx) error {
		decisionQ := uc.decision.Tx(tx)
		allocationQ := uc.checkoutDecisionAllocation.Tx(tx)
		customerProfileQ := uc.customerProfile.Tx(tx)
		creditAccountQ := uc.creditAccount.Tx(tx)
		merchantProfileQ := uc.merchantProfile.Tx(tx)
		merchantBranchProfileQ := uc.merchantBranchProfile.Tx(tx)
		installmentQ := uc.installment.Tx(tx)
		installmentChargeQ := uc.installmentCharge.Tx(tx)
		outboxQ := uc.outbox.Tx(tx)
		policyAssignmentQ := uc.policyAssignment.Tx(tx)

		// Idempotent replay must preserve the policy snapshot of the original
		// Decision even when the customer assignment changed afterwards.
		if existing, existingErr := decisionQ.GetByIdempotencyKey(ctx, input.CheckoutKey); existingErr == nil {
			existingPolicy, policyErr := uc.resolveCheckoutPolicyRef(policy.Ref{ID: existing.PolicyID, Version: existing.PolicyVersion})
			if policyErr != nil {
				return policyErr
			}

			// A replay may not switch products while reusing the same checkout key.
			// Policy is the persisted immutable source of the product identity.
			if model.EffectiveCreditKind(existingPolicy.Terms().CreditKind) != model.EffectiveCreditKind(input.CreditKind) {
				return idempotencyConflictError(fmt.Errorf("%s: checkout credit kind changed on replay", op))
			}

			fingerprint, policyErr = uc.checkoutFingerprint(
				input.CheckoutKey,
				input.ExternalSubjectID,
				input.PurchaseContext,
				input.RequestedAmountMinor,
				string(input.Currency),
				existingPolicy.Ref().ID,
				existingPolicy.Ref().Version,
			)
			if policyErr != nil {
				return policyErr
			}

			if resolvedOutput, resolved, resolveErr := uc.resolveIdempotentDecision(
				ctx,
				decisionQ,
				allocationQ,
				installmentQ,
				installmentChargeQ,
				input.CheckoutKey,
				fingerprint,
				input,
			); resolved {
				if resolveErr != nil {
					return resolveErr
				}

				output = resolvedOutput

				return nil
			}
		} else if !cerr.IsCode(existingErr, cerr.CodeResourceNotFound) {
			return existingErr
		}

		customerProfile, err := customerProfileQ.LockByExternalSubjectID(ctx, input.ExternalSubjectID)
		if err != nil {
			return err
		}

		// Trusted checkout policy assignments key off the CRS customer id, not
		// the Identity subject used to locate the profile.
		selectedPolicy, err := uc.resolveCheckoutPolicy(
			ctx,
			policyAssignmentQ,
			customerProfile.ExternalCustomerID,
			now,
		)
		// The monthly product uses a separate compiled policy and account;
		// a trusted-customer installment assignment cannot turn it into v2.
		if err == nil && model.EffectiveCreditKind(input.CreditKind) == model.PayNextMonthCreditKind {
			selectedPolicy, err = uc.checkoutPolicies.ForCreditKind(model.PayNextMonthCreditKind)
		}

		if err != nil {
			return err
		}

		fingerprint, err = uc.checkoutFingerprint(
			input.CheckoutKey,
			input.ExternalSubjectID,
			input.PurchaseContext,
			input.RequestedAmountMinor,
			string(input.Currency),
			selectedPolicy.Ref().ID,
			selectedPolicy.Ref().Version,
		)
		if err != nil {
			return err
		}

		installmentCount := selectedPolicy.Terms().InstallmentCount

		// The purchase source is the only merchant-resolution input. Missing or
		// disabled source mappings cannot yield trustworthy merchant context, so
		// propagate not-found rather than accepting merchant identity from clients.
		branchProfile, err := input.PurchaseContext.resolveMerchantBranch(ctx, merchantBranchProfileQ)
		if err != nil {
			return err
		}

		merchantProfile, err := merchantProfileQ.LockByMerchantID(
			ctx,
			branchProfile.ExternalMerchantID,
		)
		if err != nil {
			return err
		}

		// New Decision evaluation is origination. Idempotent replay of an
		// already-persisted Decision returned above and must remain possible
		// after origination is later disabled for this jurisdiction.
		if err := uc.jurisdictions.RequireOriginationEnabled(merchantProfile.Jurisdiction); err != nil {
			return cerr.InternalError(fmt.Errorf(
				"%s: merchant jurisdiction origination is not enabled: %w",
				op,
				err,
			))
		}

		// Merchant eligibility is a separate policy from customer capacity.
		// A merchant accepting installment credit must not automatically
		// accept the revolving monthly product.
		merchantEnabled, lookupErr := merchantAllowsCreditKind(
			ctx,
			merchantProfileQ,
			merchantProfile.ID,
			model.EffectiveCreditKind(input.CreditKind),
		)
		if lookupErr != nil {
			return lookupErr
		}

		if !merchantEnabled {
			merchantDenial := checkout.EligibilityResult{
				Outcome:      model.DeniedDecisionOutcome,
				ReasonCodes:  []string{model.MerchantCreditKindNotEnabledDecisionReason},
				Jurisdiction: customerProfile.Jurisdiction,
			}
			branchID := branchProfile.ID

			return uc.persistCheckoutDecision(
				ctx,
				decisionQ,
				allocationQ,
				outboxQ,
				input,
				customerProfile.ID,
				merchantProfile.ID,
				&branchID,
				fingerprint,
				selectedPolicy,
				merchantDenial,
				0,
				installmentCount,
				checkoutDecisionAuditFacts{
					ExternalSubjectID:       input.ExternalSubjectID,
					CustomerID:              customerProfile.ID,
					MerchantProfileID:       merchantProfile.ID,
					ExternalMerchantID:      merchantProfile.ExternalMerchantID,
					MerchantBranchProfileID: branchProfile.ID,
					ExternalBranchID:        branchProfile.ExternalBranchID,
					PurchaseChannel:         input.PurchaseContext.Channel,
					PurchaseSourceID:        input.PurchaseContext.sourceID(),
					MerchantOrderID:         input.PurchaseContext.merchantOrderIDPointer(),
				},
				now,
				&output,
			)
		}

		creditAccount, hasCreditAccount, err := loadCurrentCreditAccount(
			ctx,
			creditAccountQ,
			customerProfile.ID,
			input.Currency,
			now,
			model.EffectiveCreditKind(input.CreditKind),
		)
		if err != nil {
			return err
		}

		// Persistence FKs reference customer_profiles.id /
		// merchant_profiles.id / merchant_branch_profiles.id.
		customerID := customerProfile.ID
		merchantProfileID := merchantProfile.ID
		merchantBranchProfileID := branchProfile.ID

		// Fail-closed against collections default (materialized or live) after
		// the customer row is locked and before a new financing Decision can
		// approve. Idempotent replays of existing Decisions returned above.
		if guardErr := uc.newCreditUsageGuard.AssertNewCreditAllowed(
			ctx,
			tx,
			customerID,
			now,
		); guardErr != nil {
			if !cerr.IsCode(guardErr, cerr.CodeConflict) {
				return guardErr
			}

			collectionsDenial := checkout.EligibilityResult{
				Outcome:      model.DeniedDecisionOutcome,
				ReasonCodes:  []string{model.CollectionsDefaultBlocksNewCreditDecisionReason},
				Jurisdiction: customerProfile.Jurisdiction,
			}

			return uc.persistCheckoutDecision(
				ctx,
				decisionQ,
				allocationQ,
				outboxQ,
				input,
				customerID,
				merchantProfileID,
				&merchantBranchProfileID,
				fingerprint,
				selectedPolicy,
				collectionsDenial,
				0,
				installmentCount,
				checkoutDecisionAuditFacts{
					ExternalSubjectID:       input.ExternalSubjectID,
					CustomerID:              customerProfile.ID,
					MerchantProfileID:       merchantProfile.ID,
					ExternalMerchantID:      merchantProfile.ExternalMerchantID,
					MerchantBranchProfileID: branchProfile.ID,
					ExternalBranchID:        branchProfile.ExternalBranchID,
					PurchaseChannel:         input.PurchaseContext.Channel,
					PurchaseSourceID:        input.PurchaseContext.sourceID(),
					MerchantOrderID:         input.PurchaseContext.merchantOrderIDPointer(),
				},
				now,
				&output,
			)
		}

		// Unlike the general collections restriction (which may affect both
		// products), monthly cycle blocking is scoped to pay_next_month.
		// The Decision records a stable denial instead of fabricating a
		// missing credit account or zeroing the account's real exposure.
		if model.EffectiveCreditKind(input.CreditKind) == model.PayNextMonthCreditKind {
			monthlyBlocked, lookupErr := previousMonthlyCycleUnpaid(
				ctx, creditAccountQ, customerID, input.Currency, now,
			)
			if lookupErr != nil {
				return lookupErr
			}

			if monthlyBlocked {
				monthlyDenial := checkout.EligibilityResult{
					Outcome:      model.DeniedDecisionOutcome,
					ReasonCodes:  []string{model.MonthlyPreviousCycleUnpaidDecisionReason},
					Jurisdiction: customerProfile.Jurisdiction,
				}
				return uc.persistCheckoutDecision(
					ctx, decisionQ, allocationQ, outboxQ, input, customerID,
					merchantProfileID, &merchantBranchProfileID, fingerprint,
					selectedPolicy, monthlyDenial, 0, installmentCount,
					checkoutDecisionAuditFacts{
						ExternalSubjectID:       input.ExternalSubjectID,
						CustomerID:              customerProfile.ID,
						MerchantProfileID:       merchantProfile.ID,
						ExternalMerchantID:      merchantProfile.ExternalMerchantID,
						MerchantBranchProfileID: branchProfile.ID,
						ExternalBranchID:        branchProfile.ExternalBranchID,
						PurchaseChannel:         input.PurchaseContext.Channel,
						PurchaseSourceID:        input.PurchaseContext.sourceID(),
						MerchantOrderID:         input.PurchaseContext.merchantOrderIDPointer(),
					},
					now, &output,
				)
			}
		}

		prerequisiteEvaluation, evaluation := evaluateCheckoutTerms(
			selectedPolicy,
			now,
			input.RequestedAmountMinor,
			input.Currency,
			customerProfile,
			merchantProfile,
			&branchProfile,
			creditAccount,
			hasCreditAccount,
		)

		// Eligibility denials (for example merchant max-amount) still
		// expose available_credit_minor so clients can distinguish
		// "customer has credit" from "this merchant cannot accept the
		// requested amount". Persist the evaluation snapshot even for denied
		// decisions so an idempotent replay returns the original response.
		if prerequisiteEvaluation.Outcome == model.DeniedDecisionOutcome {
			return uc.persistCheckoutDecision(
				ctx,
				decisionQ,
				allocationQ,
				outboxQ,
				input,
				customerID,
				merchantProfileID,
				&merchantBranchProfileID,
				fingerprint,
				selectedPolicy,
				prerequisiteEvaluation,
				evaluation.AvailableCreditMinor,
				installmentCount,
				checkoutDecisionAuditFacts{
					ExternalSubjectID:       input.ExternalSubjectID,
					CustomerID:              customerProfile.ID,
					MerchantProfileID:       merchantProfile.ID,
					ExternalMerchantID:      merchantProfile.ExternalMerchantID,
					MerchantBranchProfileID: branchProfile.ID,
					ExternalBranchID:        branchProfile.ExternalBranchID,
					PurchaseChannel:         input.PurchaseContext.Channel,
					PurchaseSourceID:        input.PurchaseContext.sourceID(),
					MerchantOrderID:         input.PurchaseContext.merchantOrderIDPointer(),
				},
				now,
				&output,
			)
		}

		return uc.persistCheckoutDecisionWithAllocation(
			ctx,
			decisionQ,
			allocationQ,
			installmentQ,
			installmentChargeQ,
			outboxQ,
			input,
			customerID,
			merchantProfileID,
			&merchantBranchProfileID,
			fingerprint,
			selectedPolicy,
			evaluation,
			installmentCount,
			checkoutDecisionAuditFacts{
				ExternalSubjectID:       input.ExternalSubjectID,
				CustomerID:              customerID,
				MerchantProfileID:       merchantProfileID,
				ExternalMerchantID:      merchantProfile.ExternalMerchantID,
				MerchantBranchProfileID: merchantBranchProfileID,
				ExternalBranchID:        branchProfile.ExternalBranchID,
				PurchaseChannel:         input.PurchaseContext.Channel,
				PurchaseSourceID:        input.PurchaseContext.sourceID(),
				MerchantOrderID:         input.PurchaseContext.merchantOrderIDPointer(),
			},
			now,
			&output,
		)
	}

	err = uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					return i(ctx, tx)
				},
			)
		},
	)
	if err != nil {
		if isPlausibleIdempotencyCreateConflict(err) {
			replayed, resolveErr := uc.resolveCheckoutDecisionAfterConflict(ctx, input, fingerprint)
			if resolveErr != nil {
				return CreateOutput{}, resolveErr
			}

			uc.observer.DecisionReplayed(ctx, replayed.Decision.ID, replayed.Decision.Outcome)

			return replayed, nil
		}

		return CreateOutput{}, err
	}

	// Record only after a successful create commit. Idempotent replays use a
	// non-Created result and must not double-count.
	if output.Result == CreateResultCreated {
		uc.businessMetrics.CheckoutDecisionCreated(ctx, string(output.Decision.Outcome))
		uc.observeCreated(ctx, output)

		return output, nil
	}

	uc.observer.DecisionReplayed(ctx, output.Decision.ID, output.Decision.Outcome)

	return output, nil
}

// observeCreated reports a newly committed decision.
func (uc *UseCase) observeCreated(ctx context.Context, output CreateOutput) {
	decision := output.Decision

	uc.observer.AvailableCreditChecked(ctx, decision.ID, output.AvailableCreditMinor, output.Currency)

	switch decision.Outcome {
	case model.ApprovedDecisionOutcome:
		uc.observer.DecisionApproved(ctx, decision.ID)

	case model.ConditionalDecisionOutcome:
		uc.observer.DecisionPartiallyApproved(ctx, decision.ID)

	case model.DeniedDecisionOutcome:
		reasonCode := ""
		if len(decision.ReasonCodes) > 0 {
			reasonCode = decision.ReasonCodes[0]
		}

		uc.observer.DecisionDenied(ctx, decision.ID, reasonCode)
	}

	uc.observer.DecisionPersisted(ctx, decision.ID, decision.Outcome)
}

func validateCreateInput(input CreateInput) error {
	const op = "decisionuc.validateCreateInput"
	if input.CheckoutKey == "" {
		return checkoutKeyRequiredError()
	}

	if input.ExternalSubjectID.IsZero() {
		return externalSubjectIDRequiredError()
	}

	if err := input.PurchaseContext.validate(); err != nil {
		return err
	}

	if input.RequestedAmountMinor == 0 {
		return requestedAmountRequiredError()
	}

	if input.RequestedAmountMinor > math.MaxInt64 {
		return requestedAmountOutOfRangeError()
	}

	if !input.Currency.IsValid() {
		return currencyInvalidError()
	}

	if kind := model.EffectiveCreditKind(input.CreditKind); !kind.IsValid() {
		return cerr.InvalidArgument(fmt.Errorf("%s: invalid credit_kind %q", op, input.CreditKind))
	}

	return nil
}

func loadCurrentCreditAccount(
	ctx context.Context,
	accountQ repo.CreditAccountQueryer,
	customerID model.CustomerID,
	currency model.Currency,
	at time.Time,
	kind model.CreditKind,
) (model.CreditAccount, bool, error) {
	const op = "decisionuc.loadCurrentCreditAccount"
	var (
		account model.CreditAccount
		err     error
	)
	switch kind {
	case model.PayInInstallmentsCreditKind:
		account, err = accountQ.GetByCustomerIDAndCurrency(ctx, customerID, currency)

	case model.PayNextMonthCreditKind:
		productQ, ok := accountQ.(repo.CreditAccountProductQueryer)
		if !ok {
			return model.CreditAccount{}, false, cerr.InternalError(fmt.Errorf("%s: credit account repository does not support monthly products", op))
		}

		account, err = productQ.GetByCustomerIDCurrencyAndKind(ctx, customerID, currency, kind)
	default:
		return model.CreditAccount{}, false, cerr.InvalidArgument(fmt.Errorf("%s: unsupported credit kind %q", op, kind))
	}
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CreditAccount{}, false, nil
		}

		return model.CreditAccount{}, false, err
	}

	if !account.IsUsableAt(at) {
		return account, true, nil
	}

	return account, true, nil
}

func (uc *UseCase) outputFromExistingDecision(
	ctx context.Context,
	allocationQ repo.CheckoutDecisionAllocationTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	decision model.Decision,
	input CreateInput,
) (CreateOutput, error) {
	output := CreateOutput{
		Result:               CreateResultAlreadyExists,
		Decision:             decision,
		Currency:             input.Currency,
		RequestedAmountMinor: input.RequestedAmountMinor,
		ExpiresAt:            decision.ExpiresAt,
	}

	allocation, err := allocationQ.GetByDecisionID(ctx, decision.ID)
	if err != nil {
		// Pre-existing denied decisions created before denial snapshots were
		// persisted cannot reconstruct available_credit_minor. Keep their legacy
		// replay behavior while all newly created decisions persist the snapshot.
		if decision.Outcome == model.DeniedDecisionOutcome && cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return output, nil
		}

		return CreateOutput{}, err
	}

	populateAllocationOutput(&output, allocation)

	if decision.Outcome == model.DeniedDecisionOutcome {
		return output, nil
	}

	if !outcomeCreatesInstallmentOffer(decision.Outcome) {
		return output, nil
	}

	offer, action, err := uc.loadPersistedInstallmentOffer(
		ctx,
		installmentQ,
		installmentChargeQ,
		allocation,
		decision,
	)
	if err != nil {
		return CreateOutput{}, err
	}

	output.InstallmentOffer = offer
	output.CheckoutAction = action

	return output, nil
}

func (uc *UseCase) persistCheckoutDecision(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	allocationQ repo.CheckoutDecisionAllocationTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	input CreateInput,
	customerID model.CustomerID,
	merchantProfileID model.MerchantProfileID,
	merchantBranchProfileID *model.MerchantBranchProfileID,
	fingerprint string,
	selectedPolicy checkout.Policy,
	evaluation checkout.EligibilityResult,
	availableCreditMinor uint64,
	installmentCount int,
	facts checkoutDecisionAuditFacts,
	now time.Time,
	output *CreateOutput,
) error {
	expiresAt := now.Add(selectedPolicy.Terms().DecisionTTL)
	businessRule := businessrule.Active()

	decision, err := decisionQ.Create(
		ctx,
		model.ContractApprovalDecisionType,
		model.CheckoutKeyDecisionTargetType,
		model.CheckoutKeyTargetID(input.CheckoutKey),
		evaluation.Outcome,
		evaluation.ReasonCodes,
		selectedPolicy.Ref().ID,
		selectedPolicy.Ref().Version,
		evaluation.Jurisdiction,
		model.SystemDecisionActorType,
		nil,
		nil,
		nil,
		&expiresAt,
		now,
		now,
		input.CheckoutKey,
		fingerprint,
	)
	if err != nil {
		return err
	}

	allocation, err := allocationQ.Create(
		ctx,
		decision.ID,
		input.CheckoutKey,
		customerID,
		merchantProfileID,
		merchantBranchProfileID,
		input.PurchaseContext.Channel,
		input.PurchaseContext.sourceID(),
		input.PurchaseContext.merchantOrderIDPointer(),
		input.RequestedAmountMinor,
		0,
		0,
		availableCreditMinor,
		input.Currency,
		installmentCount,
		businessRule.ID(),
		businessRule.Version(),
		expiresAt,
		now,
	)
	if err != nil {
		return err
	}

	*output = CreateOutput{
		Result:   CreateResultCreated,
		Decision: decision,
	}
	populateAllocationOutput(output, allocation)

	return uc.insertCheckoutDecisionAuditOutbox(
		ctx,
		outboxQ,
		decision,
		*output,
		facts,
		now,
	)
}

func (uc *UseCase) persistCheckoutDecisionWithAllocation(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	allocationQ repo.CheckoutDecisionAllocationTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	input CreateInput,
	customerID model.CustomerID,
	merchantProfileID model.MerchantProfileID,
	merchantBranchProfileID *model.MerchantBranchProfileID,
	fingerprint string,
	selectedPolicy checkout.Policy,
	evaluation checkout.FinancingResult,
	installmentCount int,
	facts checkoutDecisionAuditFacts,
	now time.Time,
	output *CreateOutput,
) error {
	expiresAt := now.Add(selectedPolicy.Terms().DecisionTTL)
	businessRule := businessrule.Active()

	decision, err := decisionQ.Create(
		ctx,
		model.ContractApprovalDecisionType,
		model.CheckoutKeyDecisionTargetType,
		model.CheckoutKeyTargetID(input.CheckoutKey),
		evaluation.Outcome,
		evaluation.ReasonCodes,
		selectedPolicy.Ref().ID,
		selectedPolicy.Ref().Version,
		evaluation.Jurisdiction,
		model.SystemDecisionActorType,
		nil,
		nil,
		nil,
		&expiresAt,
		now,
		now,
		input.CheckoutKey,
		fingerprint,
	)
	if err != nil {
		return err
	}

	// AvailableCreditMinor is the evaluation-time snapshot before purchase commitment.
	// Approval does not reserve or reduce capacity.
	allocation, err := allocationQ.Create(
		ctx,
		decision.ID,
		input.CheckoutKey,
		customerID,
		merchantProfileID,
		merchantBranchProfileID,
		input.PurchaseContext.Channel,
		input.PurchaseContext.sourceID(),
		input.PurchaseContext.merchantOrderIDPointer(),
		input.RequestedAmountMinor,
		evaluation.FinancedAmountMinor,
		evaluation.UpfrontAmountMinor,
		evaluation.AvailableCreditMinor,
		input.Currency,
		installmentCount,
		businessRule.ID(),
		businessRule.Version(),
		expiresAt,
		now,
	)
	if err != nil {
		return err
	}

	*output = CreateOutput{
		Result:   CreateResultCreated,
		Decision: decision,
		Currency: input.Currency,
	}
	populateAllocationOutput(output, allocation)

	if outcomeCreatesInstallmentOffer(decision.Outcome) &&
		allocation.ApprovedFinancedAmountMinor > 0 {
		offer, action, offerErr := uc.createApprovedDecisionInstallments(
			ctx,
			selectedPolicy,
			installmentQ,
			installmentChargeQ,
			allocation,
			decision,
			now,
		)
		if offerErr != nil {
			return offerErr
		}

		output.InstallmentOffer = &offer
		output.CheckoutAction = &action
	}

	return uc.insertCheckoutDecisionAuditOutbox(
		ctx,
		outboxQ,
		decision,
		*output,
		facts,
		now,
	)
}

func populateAllocationOutput(
	output *CreateOutput,
	allocation model.CheckoutDecisionAllocation,
) {
	output.RequestedAmountMinor = allocation.RequestedAmountMinor
	output.ApprovedFinancedAmountMinor = allocation.ApprovedFinancedAmountMinor
	output.UpfrontAmountMinor = allocation.UpfrontAmountMinor
	output.Currency = allocation.Currency
	output.AvailableCreditMinor = allocation.AvailableCreditMinor
	output.ExpiresAt = &allocation.ExpiresAt
	output.PurchaseContext = purchaseContextFromAllocation(allocation)
}
