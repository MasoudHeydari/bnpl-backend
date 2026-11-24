//go:build development && integration

package invariants

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

	// Merely creating customer B must not give it visibility of credit A.
	var enrolledCredit uuid.UUID
	err = f.pool.Pool.QueryRow(ctx, `
SELECT credit_account_id FROM customer_credit_enrollments WHERE customer_id=$1`, secondCustomerID).Scan(&enrolledCredit)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	// B cannot obtain a second pay-in-installments credit row for the same
	// legal person, regardless of its distinct CRS and Identity identifiers.
	_, err = f.pool.Pool.Exec(ctx, `
INSERT INTO credit_accounts (customer_id, currency, credit_kind, status, effective_from)
VALUES ($1, 'IRR', 'pay_in_installments', 'active', now())`, secondCustomerID)
	var duplicateCredit *pgconn.PgError
	require.ErrorAs(t, err, &duplicateCredit)
	require.Equal(t, "uq_credit_accounts_owner_currency_kind", duplicateCredit.ConstraintName)

	// B explicitly requests access to the existing person credit.
	f.exec(t, `INSERT INTO customer_credit_enrollments
(customer_id,credit_account_id,credit_owner_id) VALUES($1,$2,$3)`, secondCustomerID, creditAccountID, ownerID)
	err = f.pool.Pool.QueryRow(ctx, `
SELECT credit_account_id FROM customer_credit_enrollments WHERE customer_id=$1`, secondCustomerID).Scan(&enrolledCredit)
	require.NoError(t, err)
	require.Equal(t, creditAccountID, enrolledCredit)

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

// TestTrustedMobileNationalIDCannotChange ensures that neither the original
// imported phone/national-ID pair nor its audit history can be rewritten.
func TestTrustedMobileNationalIDCannotChange(t *testing.T) {
	f := newFixture(t, true)
	ctx := f.ctx
	const mobile = "+989121234567"
	const nationalID = "0012345678"

	f.exec(t, `INSERT INTO trusted_mobile_numbers (mobile_e164, national_id, created_by)
VALUES ($1, $2, 'integration-test')`, mobile, nationalID)

	_, err := f.pool.Pool.Exec(ctx, `UPDATE trusted_mobile_numbers
SET national_id=$2 WHERE mobile_e164=$1`, mobile, "9876543210")
	assertViolation(t, err, "23514")

	_, err = f.pool.Pool.Exec(ctx, `DELETE FROM trusted_mobile_numbers WHERE mobile_e164=$1`, mobile)
	assertViolation(t, err, "23514")

	var persistedNationalID string
	err = f.pool.Pool.QueryRow(ctx, `SELECT national_id
FROM trusted_mobile_numbers WHERE mobile_e164=$1`, mobile).Scan(&persistedNationalID)
	require.NoError(t, err)
	require.Equal(t, nationalID, persistedNationalID)
}
