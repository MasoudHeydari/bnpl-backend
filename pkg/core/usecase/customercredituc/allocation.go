package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// resolveAllocation selects the first explicit rule: individual, group,
// default. A disabled or denied rule is decisive, not a fallback. The caller
// has verified CRS and holds the configuration and legal-person locks.
func (uc *UseCase) resolveAllocation(ctx context.Context, q repo.CreditAllocationTxQueryer, profile model.CustomerProfile, mobile, nationalCode string, kind model.CreditKind, currency model.Currency) (model.CreditAllocationRule, error) {
	byMobile, err := q.GetEligibilityByMobile(ctx, mobile)
	if err == nil && byMobile.NationalCode != nationalCode {
		return model.CreditAllocationRule{}, creditRequestIdentityMismatchError()
	}
	if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CreditAllocationRule{}, err
	}
	entry, err := q.GetEligibilityByNationalCode(ctx, nationalCode)
	if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CreditAllocationRule{}, err
	}
	if err == nil && (byMobile.ID == entry.ID || (entry.CreditOwnerID != nil && *entry.CreditOwnerID == profile.CreditOwnerID)) {
		if entry.CreditOwnerID == nil {
			if err := q.BindEligibility(ctx, entry.ID, profile.CreditOwnerID); err != nil {
				return model.CreditAllocationRule{}, err
			}
		} else if *entry.CreditOwnerID != profile.CreditOwnerID {
			return model.CreditAllocationRule{}, creditRequestIdentityMismatchError()
		}
		rule, err := q.GetRule(ctx, model.IndividualCreditAllocationScope, nil, &entry.ID, kind, currency)
		if err == nil && rule.Strategy != model.InheritCreditAllocationStrategy {
			return rule, nil
		}
		if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CreditAllocationRule{}, err
		}
		membership, err := q.GetMembership(ctx, entry.ID)
		if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CreditAllocationRule{}, err
		}
		if err == nil && membership.GroupCode != nil {
			group, err := q.GetGroup(ctx, *membership.GroupCode)
			if err != nil {
				return model.CreditAllocationRule{}, err
			}
			// A disabled group must not silently become eligible via the public rule.
			if !group.Enabled {
				return model.CreditAllocationRule{Scope: model.GroupCreditAllocationScope, Strategy: model.DisabledCreditAllocationStrategy}, nil
			}
			rule, err := q.GetRule(ctx, model.GroupCreditAllocationScope, membership.GroupCode, nil, kind, currency)
			if err == nil && rule.Strategy != model.InheritCreditAllocationStrategy {
				return rule, nil
			}
			if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return model.CreditAllocationRule{}, err
			}
		}
	}
	return q.GetRule(ctx, model.DefaultCreditAllocationScope, nil, nil, kind, currency)
}

// Fixed limits use the existing immutable policy terms as guardrails, not
// the compiled policy's default amount. Risk-based assessment will have its
// own implementation using the existing creditassessment.Client port.
func (uc *UseCase) fixedLimitAssessment(rule model.CreditAllocationRule, selected initialcreditlimit.Policy, now time.Time) (InitialLimitAssessmentResult, error) {
	terms := selected.Terms()
	if rule.Strategy != model.FixedLimitCreditAllocationStrategy || rule.Currency != terms.Currency || rule.FixedAmountMinor == 0 || rule.FixedAmountMinor > terms.MaxAmountMinor {
		return InitialLimitAssessmentResult{}, cerr.Conflict(fmt.Errorf("credit allocation rule exceeds governed policy terms"))
	}
	return normalizeAndValidateInitialLimitAssessment(InitialLimitAssessmentResult{
		Outcome: model.LimitSetDecisionOutcome, AmountMinor: rule.FixedAmountMinor, Currency: terms.Currency, ValidFor: terms.ValidFor,
		PolicyID: selected.Ref().ID, PolicyVersion: selected.Ref().Version, ReasonCodes: []string{"FIXED_CREDIT_ALLOCATION_APPROVED"},
	}, now)
}

func ruleReference(rule model.CreditAllocationRule) *uuid.UUID {
	if rule.ID == uuid.Nil {
		return nil
	}
	return &rule.ID
}
