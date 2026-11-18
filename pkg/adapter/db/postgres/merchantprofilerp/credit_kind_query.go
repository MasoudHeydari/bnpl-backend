package merchantprofilerp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// IsCreditKindEnabled is fail-closed: both pay_in_installments and pay_next_month are
// disabled until an administrator writes an explicit enabled row. Explicit
// disabled rows remain disabled.
func IsCreditKindEnabled[Q postgres.Queryer](
	ctx context.Context, q Q, id model.MerchantProfileID, kind model.CreditKind,
) (bool, error) {
	if id.IsZero() || !kind.IsValid() {
		return false, cerr.InvalidArgument(fmt.Errorf("invalid merchant or credit kind"))
	}

	type result struct {
		Enabled bool `db:"enabled"`
	}
	const query = `
SELECT COALESCE(
  (SELECT enabled FROM merchant_credit_kind_eligibilities
   WHERE merchant_profile_id=$1 AND credit_kind=$2),
  false
) AS enabled`
	value, err := postgres.QueryOne[result](
		ctx, q, "check merchant credit kind", query,
		id, kind,
	)
	if err != nil {
		return false, err
	}

	return value.Enabled, nil
}

// SetCreditKindEnabled is an explicit merchant-policy write. The transaction
// must be initiated by an authorized administration command with audit metadata.
func SetCreditKindEnabled[Q postgres.Queryer](
	ctx context.Context, q Q, id model.MerchantProfileID,
	kind model.CreditKind, enabled bool, at time.Time,
) error {
	if id.IsZero() || !kind.IsValid() || at.IsZero() {
		return cerr.InvalidArgument(fmt.Errorf("invalid merchant credit kind permission"))
	}

	const query = `
INSERT INTO merchant_credit_kind_eligibilities(
 merchant_profile_id,credit_kind,enabled,updated_at
) VALUES($1,$2,$3,$4)
ON CONFLICT(merchant_profile_id,credit_kind)
DO UPDATE SET enabled=EXCLUDED.enabled,updated_at=EXCLUDED.updated_at
`
	_, err := q.Exec(ctx, query, id, kind, enabled, at)
	if err != nil {
		return fmt.Errorf("set merchant credit kind permission: %w", postgres.MapError(err))
	}

	return nil
}
