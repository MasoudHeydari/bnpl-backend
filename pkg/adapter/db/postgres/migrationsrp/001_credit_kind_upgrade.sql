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


// Checkout product selection is immutable once a Decision is linked.
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
CREATE OR REPLACE FUNCTION bnpl.reject_claimed_installment_payment_attempt()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (
       SELECT 1 FROM bnpl.repayment_batch_installment_claims
       WHERE installment_id = NEW.installment_id
    ) THEN
        RAISE EXCEPTION 'installment already reserved for a repayment batch'
          USING ERRCODE = '23505',
                CONSTRAINT = 'uq_payment_attempts_repayment_batch_claim';
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS trg_payment_attempt_repayment_batch_claim ON payment_attempts;
CREATE TRIGGER trg_payment_attempt_repayment_batch_claim
BEFORE INSERT ON payment_attempts FOR EACH ROW
EXECUTE FUNCTION bnpl.reject_claimed_installment_payment_attempt();
