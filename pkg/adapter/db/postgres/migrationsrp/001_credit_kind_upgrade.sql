SET search_path TO bnpl;

-- Expand an existing customer/currency account into independently governed
-- product accounts. All pre-existing capacity and exposure remains pay-in-four.
ALTER TABLE credit_accounts
    ADD COLUMN IF NOT EXISTS credit_kind text NOT NULL DEFAULT 'pay_in_4';

-- Do not split, duplicate, or reassign existing financial exposure.
ALTER TABLE credit_accounts
    DROP CONSTRAINT IF EXISTS uq_credit_accounts_customer_currency;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uq_credit_accounts_customer_currency_kind') THEN
        ALTER TABLE credit_accounts ADD CONSTRAINT uq_credit_accounts_customer_currency_kind
            UNIQUE (customer_id, currency, credit_kind);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_credit_accounts_credit_kind') THEN
        ALTER TABLE credit_accounts ADD CONSTRAINT chk_credit_accounts_credit_kind
            CHECK (credit_kind IN ('pay_in_4', 'pay_next_month'));
    END IF;
END
$$;


-- Checkout product selection is immutable once a Decision is linked.
ALTER TABLE checkout_sessions
    ADD COLUMN IF NOT EXISTS credit_kind text NOT NULL DEFAULT 'pay_in_4';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_checkout_sessions_credit_kind') THEN
        ALTER TABLE checkout_sessions ADD CONSTRAINT chk_checkout_sessions_credit_kind
            CHECK (credit_kind IN ('pay_in_4', 'pay_next_month'));
    END IF;
END
$$;

-- Store an immutable customer-accepted snapshot before any external FC call.
-- Status is BNPL orchestration only; FC remains monetary/accounting authority.
CREATE TABLE IF NOT EXISTS repayment_batches (
    id uuid PRIMARY KEY,
    customer_id uuid NOT NULL REFERENCES customer_profiles(id),
    external_customer_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    snapshot jsonb NOT NULL,
    currency text NOT NULL,
    total_amount_minor bigint NOT NULL,
    status text NOT NULL DEFAULT 'created',
    financial_collection_payment_id text NULL,
    snapshot_revision bigint NOT NULL DEFAULT 0,
    next_reconciliation_at timestamptz NULL,
    last_reason_code text NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT uq_repayment_batches_customer_key UNIQUE (customer_id,idempotency_key),
    CONSTRAINT uq_repayment_batches_financial_id UNIQUE (financial_collection_payment_id),
    CONSTRAINT chk_repayment_batches_total CHECK(total_amount_minor > 0),
    CONSTRAINT chk_repayment_batches_revision CHECK(snapshot_revision >= 0 AND version > 0),
    CONSTRAINT chk_repayment_batches_currency CHECK(currency IN ('IRR','EUR','USD')),
    CONSTRAINT chk_repayment_batches_status CHECK(status IN (
       'created','prepared','starting','requires_action','reconciling',
       'accounting_pending','posted','failed','expired','manual_review'
    ))
);

-- Multiple batches must never hold the same installment concurrently.
-- The claim is retained until the corresponding BNPL application/release
-- transaction decides it is safe to remove. Reconciliation is not release.
CREATE TABLE IF NOT EXISTS repayment_batch_installment_claims (
    installment_id uuid PRIMARY KEY REFERENCES installments(id),
    batch_id uuid NOT NULL REFERENCES repayment_batches(id),
    created_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_repayment_batch_claims_batch
ON repayment_batch_installment_claims (batch_id);

CREATE INDEX IF NOT EXISTS idx_repayment_batches_reconcile_due
ON repayment_batches (next_reconciliation_at, created_at)
WHERE status IN ('created','prepared','starting','requires_action','reconciling','accounting_pending');


-- Legacy single-installment payment creation must respect batch ownership.
-- Without this guard an old API path could create a second PaymentAttempt
-- while the new CollectionPayment v2 snapshot holds an active installment.
CREATE OR REPLACE FUNCTION bnpl.reject_claimed_installment_payment_attempt ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    SET search_path = pg_catalog, bnpl
    AS $body$
BEGIN
    -- Use the same installment row lock as repayment batch creation.
    -- This closes the READ COMMITTED race in which a legacy attempt could
    -- otherwise be inserted while the batch is taking its snapshot.
    PERFORM 1 FROM installments WHERE id = NEW.installment_id FOR UPDATE;
    IF EXISTS (
       SELECT 1 FROM repayment_batch_installment_claims
       WHERE installment_id = NEW.installment_id
    ) THEN
        RAISE EXCEPTION 'installment already reserved for a repayment batch'
          USING ERRCODE = '23505',
                CONSTRAINT = 'uq_payment_attempts_repayment_batch_claim';
    END IF;
    RETURN NEW;
END
$body$;

DROP TRIGGER IF EXISTS trg_payment_attempt_repayment_batch_claim ON payment_attempts;
CREATE TRIGGER trg_payment_attempt_repayment_batch_claim
    BEFORE INSERT ON payment_attempts
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.reject_claimed_installment_payment_attempt ();


-- First StartCollectionPayment parameters are immutable across retries.
-- Multiple sessions with a different idempotency key/provider/return URL must
-- not be created after an unknown result.
ALTER TABLE repayment_batches
    ADD COLUMN IF NOT EXISTS start_idempotency_key text NULL,
    ADD COLUMN IF NOT EXISTS start_fingerprint text NULL;

ALTER TABLE repayment_batches
    ADD CONSTRAINT chk_batch_start_pair CHECK (
       (start_idempotency_key IS NULL) = (start_fingerprint IS NULL)
    );


-- Merchant-level credit-product eligibility is independent from Funding/FC
-- monetary provisioning. New merchants retain existing pay-in-four behavior.
-- Monthly credit is fail-closed until explicitly enabled for that merchant.
CREATE TABLE IF NOT EXISTS merchant_credit_product_eligibilities (
    merchant_profile_id uuid NOT NULL REFERENCES merchant_profiles(id),
    credit_kind text NOT NULL,
    enabled boolean NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (merchant_profile_id, credit_kind),
    CONSTRAINT chk_merchant_credit_product_kind
        CHECK (credit_kind IN ('pay_in_4','pay_next_month'))
);

-- An explicit monthly approval can be provisioned through the merchant
-- administration integration. Never infer it from the presence of a credit
-- account or the ability to process four installments.

-- Reviewed Runtime ACL for product-aware relations introduced above.
-- DELETE/TRUNCATE remain denied for the bnpl login; claim release is owner-side.
GRANT SELECT, INSERT ON bnpl.repayment_batches TO bnpl;
GRANT UPDATE (financial_collection_payment_id, last_reason_code, next_reconciliation_at, snapshot_revision, start_fingerprint, start_idempotency_key, status, updated_at, version) ON bnpl.repayment_batches TO bnpl;

GRANT SELECT, INSERT ON bnpl.repayment_batch_installment_claims TO bnpl;

GRANT SELECT, INSERT ON bnpl.merchant_credit_product_eligibilities TO bnpl;
GRANT UPDATE (enabled, updated_at) ON bnpl.merchant_credit_product_eligibilities TO bnpl;

-- Product selection mutates only credit_kind (+ updated_at already granted).
GRANT UPDATE (credit_kind) ON bnpl.checkout_sessions TO bnpl;
GRANT EXECUTE ON FUNCTION bnpl.reject_claimed_installment_payment_attempt () TO bnpl;
