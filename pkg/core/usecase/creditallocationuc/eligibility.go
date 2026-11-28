package creditallocationuc

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var nationalCodePattern = regexp.MustCompile(`^[0-9]{10}$`)
var mobilePattern = regexp.MustCompile(`^\+989[0-9]{9}$`)

// EligibilityInput is an unverified, canonical Iranian identity pair. A nil
// GroupCode explicitly removes group membership while preserving the person
// entry and individual rules. No CustomerProfile/CreditOwner is created.
type EligibilityInput struct {
	NationalCode string  `json:"national_code"`
	MobileE164   string  `json:"mobile_e164"`
	GroupCode    *string `json:"group_code"`
}

// EligibilityOutput is an administrative view, including all imported aliases.
type EligibilityOutput struct {
	Entry      model.CreditEligibilityEntry    `json:"entry"`
	Membership *model.CreditGroupMembership    `json:"membership"`
	Mobiles    []model.CreditEligibilityMobile `json:"mobiles"`
}

// RegisterEligibilityInTx adds an immutable identity alias and optionally
// changes the one effective membership. Both the normal importer and legacy
// Trusted importer call this operation under the exclusive configuration lock.
// PreserveMembership ensures Trusted import cannot silently replace an explicit
// group (including a previously ended membership).
func RegisterEligibilityInTx(ctx context.Context, q repo.CreditAllocationTxQueryer, input EligibilityInput, actor string, now time.Time, preserveMembership bool) (model.CreditEligibilityEntry, error) {
	if !nationalCodePattern.MatchString(input.NationalCode) {
		return model.CreditEligibilityEntry{}, invalidInput("national_code")
	}
	if !mobilePattern.MatchString(input.MobileE164) {
		return model.CreditEligibilityEntry{}, invalidInput("mobile_e164")
	}
	if !validActor(actor) {
		return model.CreditEligibilityEntry{}, invalidInput("actor")
	}
	if input.GroupCode != nil {
		if !groupCodePattern.MatchString(*input.GroupCode) {
			return model.CreditEligibilityEntry{}, invalidInput("group_code")
		}
		if _, err := q.GetGroup(ctx, *input.GroupCode); err != nil {
			return model.CreditEligibilityEntry{}, err
		}
	}
	byMobile, err := q.GetEligibilityByMobile(ctx, input.MobileE164)
	if err == nil && byMobile.NationalCode != input.NationalCode {
		return model.CreditEligibilityEntry{}, identityConflict()
	}
	if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CreditEligibilityEntry{}, err
	}
	mobileExists := err == nil
	entry, err := q.GetEligibilityByNationalCode(ctx, input.NationalCode)
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		entry, err = q.InsertEligibility(ctx, model.CreditEligibilityEntry{NationalCode: input.NationalCode, CreatedBy: actor, CreatedAt: now})
	}
	if err != nil {
		return model.CreditEligibilityEntry{}, err
	}
	if !mobileExists {
		if err := q.InsertEligibilityMobile(ctx, model.CreditEligibilityMobile{MobileE164: input.MobileE164, EligibilityID: entry.ID, CreatedBy: actor, CreatedAt: now}); err != nil {
			return model.CreditEligibilityEntry{}, err
		}
	}
	membership, err := q.GetMembership(ctx, entry.ID)
	if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CreditEligibilityEntry{}, err
	}
	if err == nil && (preserveMembership || sameGroup(membership.GroupCode, input.GroupCode)) {
		// A newly imported alias may authorize a previously pending request.
		if !mobileExists {
			err = q.QueueEligibility(ctx, &entry.ID, nil, nil, nil, now)
		}
		return entry, err
	}
	version := int64(1)
	if err == nil {
		version = membership.Version + 1
	}
	if _, err = q.PutMembership(ctx, model.CreditGroupMembership{EligibilityID: entry.ID, Version: version, GroupCode: input.GroupCode, CreatedBy: actor, CreatedAt: now}); err != nil {
		return model.CreditEligibilityEntry{}, err
	}
	if err = q.QueueEligibility(ctx, &entry.ID, nil, nil, nil, now); err != nil {
		return model.CreditEligibilityEntry{}, err
	}
	return entry, nil
}

func sameGroup(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// ImportEligibility is an atomic, bounded administrative import. Validation
// and all identity conflicts are checked before the transaction commits.
// JSON decoding belongs to the transport adapter; the core accepts typed rows.
func (uc *UseCase) ImportEligibility(ctx context.Context, rows []EligibilityInput, actor string) ([]model.CreditEligibilityEntry, error) {
	if len(rows) == 0 || len(rows) > uc.importMaxItems {
		return nil, invalidInput("rows")
	}
	if !validActor(actor) {
		return nil, invalidInput("actor")
	}
	// Reject contradictory memberships in one batch rather than letting the
	// result depend on input row order. Multiple aliases for a person are valid.
	groups := make(map[string]*string, len(rows))
	mobiles := make(map[string]string, len(rows))
	for _, row := range rows {
		if !nationalCodePattern.MatchString(row.NationalCode) {
			return nil, invalidInput("national_code")
		}
		if !mobilePattern.MatchString(row.MobileE164) {
			return nil, invalidInput("mobile_e164")
		}
		if g, ok := groups[row.NationalCode]; ok && !sameGroup(g, row.GroupCode) {
			return nil, invalidInput("group_code")
		}
		groups[row.NationalCode] = row.GroupCode
		if n, ok := mobiles[row.MobileE164]; ok && n != row.NationalCode {
			return nil, identityConflict()
		}
		mobiles[row.MobileE164] = row.NationalCode
	}
	out := make([]model.CreditEligibilityEntry, 0, len(rows))
	err := uc.tx(ctx, true, func(ctx context.Context, tx repo.Tx, q repo.CreditAllocationTxQueryer) error {
		now := uc.clock.Now().UTC()
		trustedQ := uc.trusted.Tx(tx)
		for _, row := range rows {
			trusted, err := trustedQ.GetByMobile(ctx, row.MobileE164)
			if err == nil && trusted.NationalID != row.NationalCode {
				return identityConflict()
			}

			if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return err
			}
			entry, err := RegisterEligibilityInTx(ctx, q, row, actor, now, false)
			if err != nil {
				return err
			}
			out = append(out, entry)
			if err := uc.auditChange(ctx, tx, actor, "credit_eligibility.put", "credit_eligibility", entry.ID.String(), map[string]any{"group_code": row.GroupCode}, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (uc *UseCase) GetEligibility(ctx context.Context, id uuid.UUID) (EligibilityOutput, error) {
	var out EligibilityOutput
	err := uc.tx(ctx, false, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		var err error
		out.Entry, err = q.GetEligibility(ctx, id)
		if err != nil {
			return err
		}
		membership, err := q.GetMembership(ctx, id)
		if err == nil {
			out.Membership = &membership
		} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return err
		}
		out.Mobiles, err = q.ListEligibilityMobiles(ctx, id)
		return err
	})
	return out, err
}

func (uc *UseCase) ListEligibility(ctx context.Context, group *string, after *uuid.UUID, limit int) ([]model.CreditEligibilityEntry, error) {
	if limit < 1 || limit > 500 {
		return nil, invalidInput("limit")
	}
	if group != nil && !groupCodePattern.MatchString(*group) {
		return nil, invalidInput("group_code")
	}
	var out []model.CreditEligibilityEntry
	err := uc.tx(ctx, false, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		var err error
		out, err = q.ListEligibility(ctx, group, after, limit)
		return err
	})
	return out, err
}

// IsIdentityConflict recognizes the semantic conflict used by the existing
// Trusted import's per-row audit classification.
func IsIdentityConflict(err error) bool {
	return err != nil && (cerr.IsCode(err, cerr.CodeConflict) || errors.Is(err, repo.ErrTrustedMobileNationalIDConflict))
}
