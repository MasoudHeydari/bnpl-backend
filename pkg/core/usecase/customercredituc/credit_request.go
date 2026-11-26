package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const creditRequestLease = 2 * time.Minute
const creditRequestRetry = time.Minute

func (uc *UseCase) allocationTx(ctx context.Context, fn func(context.Context, repo.Tx, repo.CreditAllocationTxQueryer) error) error {
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			allocationQ := uc.creditAllocation.Tx(tx)
			return fn(ctx, tx, allocationQ)
		})
	})
}

// requestOutput exposes only the originating customer's request summary.
func requestOutput(r model.CreditRequest) AssignInitialLimitOutput {
	summary := r.Summary()
	result := AssignInitialLimitResultPending
	if r.Status == model.RejectedCreditRequestStatus {
		result = AssignInitialLimitResultDenied
	}
	return AssignInitialLimitOutput{Result: result, CreditRequest: &summary}
}

// submitCreditRequest preserves the existing shared-credit enrollment path.
// Dormant requests are woken automatically when public assessment is enabled.
func (uc *UseCase) submitCreditRequest(ctx context.Context, input AssignInitialLimitInput) (AssignInitialLimitOutput, error) {
	kind := model.EffectiveCreditKind(input.CreditKind)
	if !kind.IsValid() {
		return AssignInitialLimitOutput{}, cerr.InvalidArgument(fmt.Errorf("invalid credit kind"))
	}
	mobile, err := normalizeAuthMobileE164(input.MobileE164)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}
	identity, err := uc.resolveActiveExternalCustomerIdentity(ctx, mobile)
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}
	now := uc.clock.Now().UTC()
	uc.observer.LimitAssignmentAttempted(ctx, input.SubjectID.AsExternalSubjectID())
	var output AssignInitialLimitOutput
	var processID uuid.UUID
	var profileID model.CustomerID
	err = uc.allocationTx(ctx, func(ctx context.Context, tx repo.Tx, q repo.CreditAllocationTxQueryer) error {
		if err := q.LockConfiguration(ctx, false); err != nil {
			return err
		}

		customerProfileQ := uc.customerProfile.Tx(tx)
		profile, err := uc.ensureCustomerProfileForInitialCredit(
			ctx,
			customerProfileQ,
			input.SubjectID.AsExternalSubjectID(),
			identity.ExternalCustomerID,
			identity.NationalCode,
			now,
		)
		if err != nil {
			return err
		}

		profileID = profile.ID
		if err := uc.lockCreditOwnerForAssignment(ctx, tx, profile); err != nil {
			return err
		}

		if !profile.IsActive() {
			return cerr.Conflict(fmt.Errorf("customer profile is not active"))
		}

		if _, err := uc.eligibleTrustedOwner(ctx, tx, profile, mobile, identity.NationalCode); err != nil {
			return err
		}
		// Imported phone/national-code conflicts also fail closed before enrollment.
		rule, err := uc.resolveAllocation(ctx, q, profile, mobile, identity.NationalCode, kind, model.IRRCurrency)
		if err != nil {
			return err
		}
		existing, err := uc.enrollExistingSharedCredit(ctx, tx, profile, model.IRRCurrency, kind, now)
		if err != nil {
			return err
		}
		if existing != nil {
			output = *existing
			output.CreditRequest, err = uc.latestCreditRequest(ctx, tx, profile.ID, kind)
			return err
		}
		open, err := q.GetOpenRequest(ctx, profile.CreditOwnerID, kind, model.IRRCurrency)
		if err == nil {
			if open.CustomerID != profile.ID {
				output = AssignInitialLimitOutput{Result: AssignInitialLimitResultPending}
				return nil
			}
			output = requestOutput(open)
			// Revalidate a changed mobile against the original request.
			if open.MobileE164 != mobile && open.QueueSource == nil {
				if err := q.QueueIdentityRecheck(ctx, open.ID, now); err != nil {
					return err
				}
				processID = open.ID
			}
			// Only a queued request is ready for processing.
			if open.QueueSource != nil && open.NextAttemptAt != nil && !open.NextAttemptAt.After(now) {
				processID = open.ID
			}
			return nil
		}
		if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return err
		}
		// A terminal explicit denial is replayed until its rule changes. A
		// mismatch may be followed by a new request with current verified identity.
		latest, err := q.GetLatestRequest(ctx, profile.ID, kind, model.IRRCurrency)
		if err == nil && latest.Status == model.RejectedCreditRequestStatus && latest.ReasonCode != model.CreditRequestIdentityMismatchReason && latest.AllocationRuleID != nil && *latest.AllocationRuleID == rule.ID && latest.MobileE164 == mobile {
			output = requestOutput(latest)
			return nil
		}
		if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return err
		}
		r := model.CreditRequest{CreditOwnerID: profile.CreditOwnerID, CustomerID: profile.ID, ExternalCustomerID: profile.ExternalCustomerID, MobileE164: mobile, NationalCode: identity.NationalCode, CreditKind: kind, Currency: model.IRRCurrency, RequestedAt: now}
		if rule.Strategy != model.DisabledCreditAllocationStrategy {
			source := model.NewCreditRequestQueueSource
			if rule.Scope != model.DefaultCreditAllocationScope {
				source = model.EligibilityCreditRequestQueueSource
			}
			r.QueueSource = &source
			r.NextAttemptAt = &now
		}
		r, err = q.InsertRequest(ctx, r)
		if err != nil {
			// Concurrent submit under the same credit owner races the open
			// unique index; surface the winner's request instead of an opaque
			// unique-violation internal error.
			if cerr.IsCode(err, cerr.CodeConflict) {
				open, openErr := q.GetOpenRequest(ctx, profile.CreditOwnerID, kind, model.IRRCurrency)
				if openErr != nil {
					return err
				}

				if open.CustomerID != profile.ID {
					output = AssignInitialLimitOutput{Result: AssignInitialLimitResultPending}
					return nil
				}

				output = requestOutput(open)
				return nil
			}

			return err
		}

		output = requestOutput(r)
		output.RequestCreated = true
		if r.QueueSource != nil {
			processID = r.ID
		}

		return nil
	})
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}
	if processID != uuid.Nil {
		processed, err := uc.processCreditRequest(ctx, processID)
		if err != nil {
			return AssignInitialLimitOutput{}, err
		}
		if processed.Result != "" {
			processed.RequestCreated = output.RequestCreated
			output = processed
		}
	}
	uc.observeAssignment(ctx, profileID, output)
	return output, nil
}

// ProcessDueCreditRequests is the bounded background entry point. A lease
// fences concurrent workers, and expiration makes interrupted work retryable.
func (uc *UseCase) ProcessDueCreditRequests(ctx context.Context, limit int) error {
	if limit < 1 || limit > 1000 {
		return cerr.InvalidArgument(fmt.Errorf("invalid credit request batch size"))
	}
	var rows []model.CreditRequest
	err := uc.allocationTx(ctx, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		var err error
		if err := q.LockConfiguration(ctx, false); err != nil {
			return err
		}
		now := uc.clock.Now().UTC()
		// Avoid scanning dormant requests while both public products are off.
		// These reads share the same configuration fence as the queue operation.
		for _, kind := range []model.CreditKind{model.PayInInstallmentsCreditKind, model.PayNextMonthCreditKind} {
			rule, err := q.GetRule(ctx, model.DefaultCreditAllocationScope, nil, nil, kind, model.IRRCurrency)
			if err != nil {
				return err
			}
			if rule.Strategy == model.RiskBasedCreditAllocationStrategy {
				if err := q.QueuePublicRequests(ctx, now, limit); err != nil {
					return err
				}
				break
			}
		}
		rows, err = q.ListDueRequests(ctx, now, limit)
		return err
	})
	if err != nil {
		return err
	}
	var firstErr error
	for _, r := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := uc.processCreditRequest(ctx, r.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (uc *UseCase) processCreditRequest(ctx context.Context, id uuid.UUID) (AssignInitialLimitOutput, error) {
	var claimed model.CreditRequest
	now := uc.clock.Now().UTC()
	err := uc.allocationTx(ctx, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		var err error
		claimed, err = q.ClaimRequest(ctx, id, now, now.Add(creditRequestLease))
		return err
	})
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return AssignInitialLimitOutput{}, nil
	}
	if err != nil {
		return AssignInitialLimitOutput{}, err
	}
	// Always resolve the ORIGINAL phone; a new gateway mobile must not replace
	// the request's identity evidence. Network I/O never runs inside a TX.
	resolved, resolveErr := uc.customerResolver.ResolveByMobile(ctx, extcustomer.ResolveByMobileInput{MobileE164: claimed.MobileE164})
	if cerr.IsCode(resolveErr, cerr.CodeResourceNotFound) || (resolveErr == nil && crsNationalCodePattern.MatchString(resolved.NationalCode) && !resolved.ExternalCustomerID.IsZero() && (resolved.NationalCode != claimed.NationalCode || resolved.ExternalCustomerID != claimed.ExternalCustomerID)) {
		return uc.finishRequestWithoutGrant(ctx, claimed, model.RejectedCreditRequestStatus, model.CreditRequestIdentityMismatchReason, nil)
	}
	if resolveErr != nil || resolved.Status != extcustomer.ActiveStatus || resolved.ExternalCustomerID.IsZero() || !crsNationalCodePattern.MatchString(resolved.NationalCode) {
		retry := uc.clock.Now().UTC().Add(creditRequestRetry)
		return uc.finishRequestWithoutGrant(ctx, claimed, model.PendingCreditRequestStatus, "IDENTITY_REVALIDATION_DEFERRED", &retry)
	}
	return uc.finalizeCreditRequest(ctx, claimed)
}

func (uc *UseCase) finishRequestWithoutGrant(ctx context.Context, r model.CreditRequest, status model.CreditRequestStatus, reason string, next *time.Time) (AssignInitialLimitOutput, error) {
	var output AssignInitialLimitOutput
	err := uc.allocationTx(ctx, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		current, err := q.FinishRequest(ctx, r, status, reason, nil, nil, next, uc.clock.Now().UTC())
		if err != nil {
			return err
		}
		output = requestOutput(current)
		return nil
	})
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return AssignInitialLimitOutput{}, nil
	}
	return output, err
}

// finalizeCreditRequest holds the current configuration in a shared lock
// and the person in an exclusive lock through Decision, account, journal,
// request completion and audit outbox. Configuration cannot change between
// selecting the rule and committing a fixed limit.
func (uc *UseCase) finalizeCreditRequest(ctx context.Context, claimed model.CreditRequest) (AssignInitialLimitOutput, error) {
	var output AssignInitialLimitOutput
	err := uc.allocationTx(ctx, func(ctx context.Context, tx repo.Tx, q repo.CreditAllocationTxQueryer) error {
		if err := q.LockConfiguration(ctx, false); err != nil {
			return err
		}
		sharedCreditQ := uc.sharedCredit.Tx(tx)
		owner, err := sharedCreditQ.LockOwnerByCustomer(ctx, claimed.CustomerID)
		if err != nil {
			return err
		}

		customerProfileQ := uc.customerProfile.Tx(tx)
		profile, err := customerProfileQ.LockByID(ctx, claimed.CustomerID)
		if err != nil {
			return err
		}

		r, err := q.GetRequest(ctx, claimed.ID, true)
		if err != nil {
			return err
		}
		now := uc.clock.Now().UTC()
		if r.Status != model.ProcessingCreditRequestStatus || r.LeaseToken == nil || claimed.LeaseToken == nil || *r.LeaseToken != *claimed.LeaseToken || r.LeaseUntil == nil || !r.LeaseUntil.After(now) {
			return nil
		}
		finish := func(status model.CreditRequestStatus, reason string, decision *model.DecisionID, rule *uuid.UUID, next *time.Time) error {
			completed, err := q.FinishRequest(ctx, r, status, reason, decision, rule, next, now)
			if err != nil {
				return err
			}
			summary := completed.Summary()
			if output.Result == "" {
				output = requestOutput(completed)
			} else {
				output.CreditRequest = &summary
			}
			return nil
		}
		if owner != r.CreditOwnerID || profile.CreditOwnerID != owner || profile.ExternalCustomerID != r.ExternalCustomerID {
			return finish(model.RejectedCreditRequestStatus, model.CreditRequestIdentityMismatchReason, nil, nil, nil)
		}
		if !profile.IsActive() {
			retry := now.Add(creditRequestRetry)
			return finish(model.PendingCreditRequestStatus, "CUSTOMER_NOT_ACTIVE", nil, nil, &retry)
		}
		trusted, err := uc.eligibleTrustedOwner(ctx, tx, profile, r.MobileE164, r.NationalCode)
		if cerr.IsCode(err, cerr.CodeConflict) {
			return finish(model.RejectedCreditRequestStatus, model.CreditRequestIdentityMismatchReason, nil, nil, nil)
		}
		if err != nil {
			return err
		}
		rule, err := uc.resolveAllocation(ctx, q, profile, r.MobileE164, r.NationalCode, r.CreditKind, r.Currency)
		if cerr.IsCode(err, cerr.CodeConflict) {
			return finish(model.RejectedCreditRequestStatus, model.CreditRequestIdentityMismatchReason, nil, nil, nil)
		}
		if err != nil {
			return err
		}
		// Existing capacity can be reused only by the original authenticated
		// request account. Another account cannot renew expired person credit.
		shared, err := uc.enrollExistingSharedCredit(ctx, tx, profile, r.Currency, r.CreditKind, now)
		if err != nil {
			return err
		}
		if shared != nil {
			output = *shared
			return finish(model.ApprovedCreditRequestStatus, "", &shared.Decision.ID, nil, nil)
		}
		// Resolve current rules regardless of submission time or queue source.
		if r.QueueSource == nil || rule.Strategy == model.DisabledCreditAllocationStrategy {
			return finish(model.PendingCreditRequestStatus, "ALLOCATION_DEFERRED", nil, ruleReference(rule), nil)
		}
		if rule.Strategy == model.DenyCreditAllocationStrategy {
			return finish(model.RejectedCreditRequestStatus, "CREDIT_ALLOCATION_DENIED", nil, ruleReference(rule), nil)
		}
		// No placeholder scoring or default amount is allowed in the risk-based
		// path. A future assessor must execute outside this TX and recheck its rule.
		if rule.Strategy != model.FixedLimitCreditAllocationStrategy || rule.Scope == model.DefaultCreditAllocationScope {
			return finish(model.PendingCreditRequestStatus, "ASSESSOR_NOT_AVAILABLE", nil, ruleReference(rule), nil)
		}
		var selected initialcreditlimit.Policy
		if r.CreditKind == model.PayNextMonthCreditKind {
			selected, err = uc.initialLimitPolicies.Resolve(uc.monthlyLimitRef)
		} else if trusted {
			selected, err = uc.trustedInstallmentPolicy()
		} else {
			policyAssignmentQ := uc.policyAssignment.Tx(tx)
			selected, err = uc.resolveInitialLimitPolicy(ctx, policyAssignmentQ, profile.ExternalCustomerID, now)
		}
		if err != nil {
			return err
		}
		assessment, err := uc.fixedLimitAssessment(rule, selected, now)
		if err != nil {
			return err
		}
		creditAccountQ := uc.creditAccount.Tx(tx)
		accountQ, ok := creditAccountQ.(repo.CreditAccountProductTxQueryer)
		if !ok {
			return cerr.InternalError(fmt.Errorf("product credit repository is required"))
		}

		if err := accountQ.ExpireDueByCustomerIDCurrencyAndKind(ctx, profile.ID, r.Currency, r.CreditKind, now); err != nil {
			return err
		}

		account, getErr := accountQ.LockByCustomerIDCurrencyAndKind(ctx, profile.ID, r.Currency, r.CreditKind)
		if getErr != nil && !cerr.IsCode(getErr, cerr.CodeResourceNotFound) {
			return getErr
		}
		if getErr == nil && account.Status == model.RevokedCreditAccountStatus {
			return finish(model.RejectedCreditRequestStatus, "CREDIT_ACCOUNT_REVOKED", nil, ruleReference(rule), nil)
		}
		// The request ID, not a mutable policy or account cycle, identifies the
		// immutable Decision. Completion and Decision are in the same transaction.
		key := "credit-request:" + r.ID.String()
		fingerprint, err := uc.fingerprintHasher.Hash(fmt.Sprintf("credit-request:v1:%s:%s:%s:%d:%s:%s", r.ID, rule.ID, r.CreditKind, assessment.AmountMinor, assessment.PolicyID, assessment.PolicyVersion))
		if err != nil {
			return err
		}
		if err := fingerprint.Validate(); err != nil {
			return err
		}
		decisionQ := uc.decision.Tx(tx)
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
			key,
			fingerprint.String(),
		)
		if err != nil {
			return err
		}
		expires := now.Add(assessment.ValidFor)
		previous := account
		if getErr == nil {
			account, err = accountQ.SetLimit(ctx, account.ID, assessment.AmountMinor, decision.ID, now, &expires, now)
		} else {
			account, err = accountQ.CreateWithLimitForKind(ctx, profile.ID, r.Currency, r.CreditKind, assessment.AmountMinor, decision.ID, now, &expires, now, now)
		}
		if err != nil {
			return err
		}
		journalType, journalAmount := limitChangeJournal(previous, getErr == nil, assessment.AmountMinor)
		if _, err = accountQ.AppendTransaction(ctx, account.ID, journalType, journalAmount, r.Currency, model.CreditAssessmentCreditTransactionReason, model.DecisionCreditTransactionReferenceType, decision.ID.UUID(), now, now); err != nil {
			return err
		}
		// Keep Trusted checkout behavior independent of group membership. An
		// ordinary manager/VIP group must never grant the Trusted checkout bundle.
		if trusted && r.CreditKind == model.PayInInstallmentsCreditKind {
			policyAssignmentQ := uc.policyAssignment.Tx(tx)
			if err := grantTrustedOwnerPolicies(ctx, policyAssignmentQ, owner, now); err != nil {
				return err
			}
		}

		output = AssignInitialLimitOutput{Result: AssignInitialLimitResultCreated, Decision: decision, CreditLimit: &account}
		if err := finish(model.ApprovedCreditRequestStatus, "", &decision.ID, ruleReference(rule), nil); err != nil {
			return err
		}

		// TODO: After agreeing on the credit-granted notification types and
		// template variables with Notification, insert its command in this TX.
		// Do not send on pending, replay or shared-credit enrollment.
		outboxQ := uc.outbox.Tx(tx)
		return uc.insertAssignInitialLimitAuditOutbox(ctx, outboxQ, profile, output, assessment, now)
	})
	return output, err
}

// latestCreditRequest is a customer-scoped read; another account sharing
// the same person never receives the originator's identity or request ID.
func (uc *UseCase) latestCreditRequest(ctx context.Context, tx repo.Tx, customer model.CustomerID, kind model.CreditKind) (*model.CreditRequestSummary, error) {
	allocationQ := uc.creditAllocation.Tx(tx)
	r, err := allocationQ.GetLatestRequest(ctx, customer, kind, model.IRRCurrency)
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	s := r.Summary()
	return &s, nil
}
