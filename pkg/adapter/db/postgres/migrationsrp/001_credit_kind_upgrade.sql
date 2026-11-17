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
