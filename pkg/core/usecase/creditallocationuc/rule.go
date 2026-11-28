package creditallocationuc

import (
	"context"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// RuleInput describes desired configuration, not a direct account mutation.
type RuleInput struct {
	Scope            model.CreditAllocationScope    `json:"scope" binding:"required" enums:"group,individual"`
	GroupCode        *string                        `json:"group_code"`
	EligibilityID    *uuid.UUID                     `json:"eligibility_id"`
	CreditKind       model.CreditKind               `json:"credit_kind" binding:"required" enums:"pay_in_installments,pay_next_month"`
	Currency         model.Currency                 `json:"currency" binding:"required" enums:"IRR"`
	Strategy         model.CreditAllocationStrategy `json:"strategy" binding:"required" enums:"disabled,fixed_limit,deny,inherit"`
	FixedAmountMinor uint64                         `json:"fixed_amount_minor"`
}

func (uc *UseCase) validateRule(input RuleInput) error {
	if err := validateCurrency(input.Currency); err != nil {
		return err
	}
	if !input.CreditKind.IsValid() {
		return invalidInput("credit_kind")
	}
	switch input.Scope {
	case model.DefaultCreditAllocationScope:
		if input.GroupCode != nil || input.EligibilityID != nil || (input.Strategy != model.DisabledCreditAllocationStrategy && input.Strategy != model.RiskBasedCreditAllocationStrategy) {
			return invalidInput("scope")
		}
	case model.GroupCreditAllocationScope:
		if input.GroupCode == nil || !groupCodePattern.MatchString(*input.GroupCode) || input.EligibilityID != nil {
			return invalidInput("group_code")
		}
	case model.IndividualCreditAllocationScope:
		if input.EligibilityID == nil || *input.EligibilityID == uuid.Nil || input.GroupCode != nil {
			return invalidInput("eligibility_id")
		}
	default:
		return invalidInput("scope")
	}
	switch input.Strategy {
	case model.FixedLimitCreditAllocationStrategy:
		terms := uc.installments.Terms()
		if input.CreditKind == model.PayNextMonthCreditKind {
			terms = uc.monthly.Terms()
		}
		if model.ValidatePositiveAmountMinor(input.FixedAmountMinor) != nil || input.FixedAmountMinor > terms.MaxAmountMinor {
			return invalidInput("fixed_amount_minor")
		}
	case model.DisabledCreditAllocationStrategy, model.DenyCreditAllocationStrategy, model.InheritCreditAllocationStrategy:
		if input.FixedAmountMinor != 0 {
			return invalidInput("fixed_amount_minor")
		}
	case model.RiskBasedCreditAllocationStrategy:
		return unsupportedStrategy()
	default:
		return invalidInput("strategy")
	}
	return nil
}

// PutRule appends an immutable version if the desired configuration changed.
// This endpoint is limited to named scopes; default configuration has its
// own command, which never accepts a caller-supplied credit amount.
func (uc *UseCase) PutRule(ctx context.Context, input RuleInput, actor string) (model.CreditAllocationRule, error) {
	if input.Scope == model.DefaultCreditAllocationScope {
		return model.CreditAllocationRule{}, invalidInput("scope")
	}
	return uc.putRule(ctx, input, actor)
}

func (uc *UseCase) putRule(ctx context.Context, input RuleInput, actor string) (model.CreditAllocationRule, error) {
	if err := uc.validateRule(input); err != nil {
		return model.CreditAllocationRule{}, err
	}
	if !validActor(actor) {
		return model.CreditAllocationRule{}, invalidInput("actor")
	}
	var out model.CreditAllocationRule
	err := uc.tx(ctx, true, func(ctx context.Context, tx repo.Tx, q repo.CreditAllocationTxQueryer) error {
		if input.GroupCode != nil {
			if _, err := q.GetGroup(ctx, *input.GroupCode); err != nil {
				return err
			}
		}
		if input.EligibilityID != nil {
			if _, err := q.GetEligibility(ctx, *input.EligibilityID); err != nil {
				return err
			}
		}
		current, err := q.GetRule(ctx, input.Scope, input.GroupCode, input.EligibilityID, input.CreditKind, input.Currency)
		if err == nil && current.Strategy == input.Strategy && current.FixedAmountMinor == input.FixedAmountMinor {
			out = current
			return nil
		}
		if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return err
		}
		version := int64(1)
		if err == nil {
			version = current.Version + 1
		}
		now := uc.clock.Now().UTC()
		out, err = q.PutRule(ctx, model.CreditAllocationRule{Scope: input.Scope, GroupCode: input.GroupCode, EligibilityID: input.EligibilityID, CreditKind: input.CreditKind, Currency: input.Currency, Strategy: input.Strategy, FixedAmountMinor: input.FixedAmountMinor, Version: version, CreatedBy: actor, CreatedAt: now})
		if err != nil {
			return err
		}
		if input.Scope != model.DefaultCreditAllocationScope {
			if err := q.QueueEligibility(ctx, input.EligibilityID, input.GroupCode, &input.CreditKind, &input.Currency, now); err != nil {
				return err
			}
		}
		return uc.auditChange(ctx, tx, actor, "credit_allocation_rule.put", "credit_allocation_rule", out.ID.String(), map[string]any{"scope": out.Scope, "strategy": out.Strategy, "credit_kind": out.CreditKind, "currency": out.Currency, "version": out.Version, "fixed_amount_minor": out.FixedAmountMinor}, now)
	})
	return out, err
}

func (uc *UseCase) listAllRules(ctx context.Context) ([]model.CreditAllocationRule, error) {
	var out []model.CreditAllocationRule
	err := uc.tx(ctx, false, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		var err error
		out, err = q.ListRules(ctx)
		return err
	})
	return out, err
}

// DefaultInput switches the public risk assessment path on or off. It has no
// amount field: the future risk algorithm owns the approved amount.
type DefaultInput struct {
	CreditKind model.CreditKind `json:"credit_kind" binding:"required" enums:"pay_in_installments,pay_next_month"`
	Currency   model.Currency   `json:"currency" binding:"required" enums:"IRR"`
	Enabled    *bool            `json:"enabled" binding:"required"`
}

func (uc *UseCase) PutDefault(ctx context.Context, input DefaultInput, actor string) (model.CreditAllocationRule, error) {
	if input.Enabled == nil {
		return model.CreditAllocationRule{}, invalidInput("enabled")
	}
	strategy := model.DisabledCreditAllocationStrategy
	if *input.Enabled {
		strategy = model.RiskBasedCreditAllocationStrategy
	}
	// validateRule rejects risk_based until a real assessor is registered.
	return uc.putRule(ctx, RuleInput{Scope: model.DefaultCreditAllocationScope, CreditKind: input.CreditKind, Currency: input.Currency, Strategy: strategy}, actor)
}

func (uc *UseCase) ListDefaults(ctx context.Context) ([]model.CreditAllocationRule, error) {
	return uc.listRulesByScope(ctx, true)
}

func (uc *UseCase) ListRules(ctx context.Context) ([]model.CreditAllocationRule, error) {
	return uc.listRulesByScope(ctx, false)
}

func (uc *UseCase) listRulesByScope(ctx context.Context, defaults bool) ([]model.CreditAllocationRule, error) {
	rows, err := uc.listAllRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.CreditAllocationRule, 0, len(rows))
	for _, row := range rows {
		if (row.Scope == model.DefaultCreditAllocationScope) == defaults {
			out = append(out, row)
		}
	}
	return out, nil
}

func validateCurrency(currency model.Currency) error {
	if !currency.IsValid() {
		return invalidInput("currency")
	}
	if currency != model.IRRCurrency {
		return unsupportedCurrency()
	}
	return nil
}
