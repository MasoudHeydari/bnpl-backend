package creditaccountrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// validateProduct prevents a malformed product from selecting an arbitrary
// account or inserting an untyped credit limit.
func validateProduct(kind model.CreditKind) error {
	if !kind.IsValid() {
		return cerr.InvalidArgument(fmt.Errorf("unsupported credit kind %q", kind))
	}
	return nil
}

func GetByCustomerIDCurrencyAndKind[Q postgres.Queryer](
	ctx context.Context, q Q, customerID model.CustomerID, currency model.Currency, kind model.CreditKind,
) (model.CreditAccount, error) {
	if err := validateProduct(kind); err != nil {
		return model.CreditAccount{}, err
	}
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts
WHERE customer_id = $1 AND currency = $2 AND credit_kind = $3
`
	return postgres.QueryOne[model.CreditAccount](ctx, q, "get credit account by product", query, customerID, currency, kind)
}

func LockByCustomerIDCurrencyAndKind[Q postgres.Queryer](
	ctx context.Context, q Q, customerID model.CustomerID, currency model.Currency, kind model.CreditKind,
) (model.CreditAccount, error) {
	if err := validateProduct(kind); err != nil {
		return model.CreditAccount{}, err
	}
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts
WHERE customer_id = $1 AND currency = $2 AND credit_kind = $3
FOR UPDATE
`
	return postgres.QueryOne[model.CreditAccount](ctx, q, "lock credit account by product", query, customerID, currency, kind)
}

// ListByCustomerID exposes each product separately; callers must not add
// limit or available balances across products as if they were fungible.
func ListByCustomerID[Q postgres.Queryer](ctx context.Context, q Q, customerID model.CustomerID) ([]model.CreditAccount, error) {
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts WHERE customer_id = $1 ORDER BY currency,credit_kind
`
	return postgres.QueryAll[model.CreditAccount](ctx, q, "list credit accounts by customer", query, customerID)
}

func CreateWithLimitForKind[Q postgres.Queryer](
	ctx context.Context, q Q, customerID model.CustomerID, currency model.Currency, kind model.CreditKind,
	limitAmountMinor uint64, decisionID model.DecisionID, effectiveFrom time.Time, expiresAt *time.Time, createdAt time.Time, updatedAt time.Time,
) (model.CreditAccount, error) {
	if err := validateProduct(kind); err != nil {
		return model.CreditAccount{}, err
	}
	if err := postgres.RequiredTimes(map[string]time.Time{"effective_from": effectiveFrom, "created_at": createdAt, "updated_at": updatedAt}); err != nil {
		return model.CreditAccount{}, err
	}
	const query = `
INSERT INTO credit_accounts(
 customer_id,currency,credit_kind,limit_amount_minor,exposure_amount_minor,status,
 last_limit_decision_id,effective_from,expires_at,revoked_at,version,created_at,updated_at
) VALUES($1,$2,$3,$4,0,'active',$5,$6,$7,NULL,1,$8,$9)
RETURNING` + creditAccountColumns
	row, err := postgres.QueryOne[model.CreditAccount](ctx, q, "create product credit account", query,
		customerID, currency, kind, limitAmountMinor, decisionID, effectiveFrom, expiresAt, createdAt, updatedAt)
	if err != nil && postgres.IsUniqueViolation(err, "uq_credit_accounts_customer_currency_kind") {
		return model.CreditAccount{}, cerr.Conflict(fmt.Errorf("credit account already exists for customer, currency and kind"),
			map[string]any{"customer_id": customerID, "currency": currency, "credit_kind": kind})
	}
	return row, err
}

func ExpireDueByCustomerIDCurrencyAndKind[Q postgres.Queryer](
	ctx context.Context, q Q, customerID model.CustomerID, currency model.Currency, kind model.CreditKind, now time.Time,
) error {
	if err := validateProduct(kind); err != nil {
		return err
	}
	const query = `
UPDATE credit_accounts SET status='expired',version=version+1,updated_at=$4
WHERE customer_id=$1 AND currency=$2 AND credit_kind=$3
 AND status='active' AND expires_at IS NOT NULL AND expires_at <= $4
`
	_, err := q.Exec(ctx, query, customerID, currency, kind, now)
	if err != nil {
		return fmt.Errorf("expire product credit account: %w", postgres.MapError(err))
	}
	return nil
}

// HasUnpaidMonthlyCycleBefore blocks new next-month purchases from the start
// of a new Jalali month while an installment from an earlier cycle remains
// unpaid. It counts only activated monthly contracts and never treats
// pending checkout offers or four-installment obligations as monthly debt.
func HasUnpaidMonthlyCycleBefore[Q postgres.Queryer](
	ctx context.Context, q Q, customerID model.CustomerID, currency model.Currency, cutoff time.Time,
) (bool, error) {
	if cutoff.IsZero() {
		return false, cerr.InvalidArgument(fmt.Errorf("monthly cutoff is required"))
	}
	type result struct {
		Blocked bool `db:"blocked"`
	}
	const query = `
SELECT EXISTS(
	SELECT 1
	FROM installments AS i
	JOIN contracts AS c ON c.id=i.contract_id
	JOIN credit_accounts AS ca ON ca.id=c.credit_account_id
	WHERE ca.customer_id=$1
	  AND ca.currency=$2
	  AND ca.credit_kind='pay_next_month'
	  AND c.status='active'
	  AND i.status IN ('scheduled','due','overdue')
	  AND i.due_at < $3
) AS blocked
`
	row, err := postgres.QueryOne[result](ctx, q, "find unpaid previous monthly cycle", query, customerID, currency, cutoff)
	if err != nil {
		return false, err
	}
	return row.Blocked, nil
}
