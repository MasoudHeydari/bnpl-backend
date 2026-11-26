package creditallocationuc

import (
	"context"
	"regexp"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var groupCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// PutGroup creates or replaces group metadata. Repeating the same desired
// state is a no-op. Only pending requests are queued on enablement.
func (uc *UseCase) PutGroup(ctx context.Context, code, name string, enabled bool, actor string) (model.CreditGroup, error) {
	name = strings.TrimSpace(name)
	if !groupCodePattern.MatchString(code) {
		return model.CreditGroup{}, invalidInput("code")
	}
	if len(name) == 0 || len([]rune(name)) > 128 {
		return model.CreditGroup{}, invalidInput("name")
	}
	if !validActor(actor) {
		return model.CreditGroup{}, invalidInput("actor")
	}
	var out model.CreditGroup
	err := uc.tx(ctx, true, func(ctx context.Context, tx repo.Tx, q repo.CreditAllocationTxQueryer) error {
		existing, err := q.GetGroup(ctx, code)
		if err == nil && existing.Name == name && existing.Enabled == enabled {
			out = existing
			return nil
		}
		if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return err
		}
		now := uc.clock.Now().UTC()
		out, err = q.PutGroup(ctx, model.CreditGroup{Code: code, Name: name, Enabled: enabled, UpdatedBy: actor, UpdatedAt: now})
		if err != nil {
			return err
		}
		if enabled {
			if err := q.QueueEligibility(ctx, nil, &code, nil, nil, now); err != nil {
				return err
			}
		}
		return uc.auditChange(ctx, tx, actor, "credit_group.put", "credit_group", code, map[string]any{"enabled": enabled, "name": name}, now)
	})
	return out, err
}

func (uc *UseCase) ListGroups(ctx context.Context) ([]model.CreditGroup, error) {
	var out []model.CreditGroup
	err := uc.tx(ctx, false, func(ctx context.Context, _ repo.Tx, q repo.CreditAllocationTxQueryer) error {
		var err error
		out, err = q.ListGroups(ctx)
		return err
	})
	return out, err
}

func validActor(actor string) bool { return strings.TrimSpace(actor) != "" && len(actor) <= 256 }
