//go:build development && integration

package invariants

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// TestCreditOwnerCheckoutClaimIsExclusive proves that two phone accounts for
// one national code cannot own concurrent hosted credit checkouts, even if the
// products are different. The uniqueness guarantee comes from PostgreSQL.
func TestCreditOwnerCheckoutClaimIsExclusive(t *testing.T) {
	f := newFixture(t, true)
	ctx := f.ctx
	var ownerID, externalMerchantID, merchantID uuid.UUID
	err := f.pool.Pool.QueryRow(ctx, `
SELECT cp.credit_owner_id, mp.external_merchant_id, mp.id
FROM customer_profiles cp
CROSS JOIN merchant_profiles mp
WHERE cp.id=$1 LIMIT 1`, f.customer).Scan(&ownerID, &externalMerchantID, &merchantID)
	require.NoError(t, err)

	branchID := uuid.New()
	f.exec(t, `INSERT INTO merchant_branch_profiles
(id,external_merchant_id,external_branch_id,branch_type,status,jurisdiction)
VALUES($1,$2,$3,'online','active','IR')`, branchID, externalMerchantID, uuid.New())

	secondCustomerID := uuid.New()
	f.exec(t, `INSERT INTO customer_profiles
(id,external_subject_id,external_customer_id,credit_owner_id,jurisdiction,status)
VALUES($1,$2,$3,$4,'IR','active')`, secondCustomerID, uuid.New(), uuid.New(), ownerID)
	var creditAccountID uuid.UUID
	err = f.pool.Pool.QueryRow(ctx, `
SELECT id FROM credit_accounts WHERE customer_id=$1`, f.customer).Scan(&creditAccountID)
	require.NoError(t, err)
	f.exec(t, `INSERT INTO customer_credit_enrollments
(customer_id,credit_account_id,credit_owner_id) VALUES($1,$2,$3)`, secondCustomerID, creditAccountID, ownerID)

	newCheckout := func(customerID uuid.UUID, label string) error {
		const op = "invariants.TestCreditOwnerCheckoutClaimIsExclusive.newCheckout"
		publicToken := "cs_" + strings.Repeat(label, 64)
		_, err := f.pool.Pool.Exec(ctx, `
INSERT INTO checkout_sessions (
 public_token,origin,merchant_profile_id,merchant_branch_profile_id,
 external_online_store_id,idempotency_key,merchant_order_id,customer_id,
 return_destination,return_url,requested_amount_minor,currency,status,expires_at,credit_kind
) VALUES($1,'external_partner',$2,$3,$4,$5,$6,$7,'merchant','https://example.test/return',
 1000000,'IRR','open',now()+interval '30 minutes','pay_in_installments')`,
			publicToken,
			merchantID,
			branchID,
			uuid.New(),
			fmt.Sprintf("checkout-%s", label),
			fmt.Sprintf("order-%s", label),
			customerID,
		)
		if err != nil {
			return fmt.Errorf("%s: insert checkout session: %w", op, err)
		}

		return nil
	}

	require.NoError(t, newCheckout(f.customer, "a"))

	// Even the schema owner must not be able to release a live claim by an
	// UPDATE that bypasses the hosted Checkout state transition.
	_, err = f.pool.Pool.Exec(ctx, `UPDATE credit_owner_checkout_claims
SET released_at = now() WHERE credit_owner_id = $1 AND released_at IS NULL`, ownerID)
	assertViolation(t, err, "23514")

	err = newCheckout(secondCustomerID, "b")
	require.Error(t, err)
	var pgError *pgconn.PgError
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, "23505", pgError.Code)
	require.Equal(t, "credit_owner_checkout_in_progress", pgError.ConstraintName)

	// Once the first session reaches a genuine terminal status, the existing
	// claim is released. A different account may then proceed independently.
	f.exec(t, `UPDATE checkout_sessions SET status='expired'
WHERE public_token=$1`, "cs_"+strings.Repeat("a", 64))

	var releasedCount int
	err = f.pool.Pool.QueryRow(ctx, `SELECT count(*) FROM credit_owner_checkout_claims
WHERE credit_owner_id=$1 AND released_at IS NOT NULL`, ownerID).Scan(&releasedCount)
	require.NoError(t, err)
	require.Equal(t, 1, releasedCount, "terminal claim history must be retained")
	require.NoError(t, newCheckout(secondCustomerID, "b"))
}
