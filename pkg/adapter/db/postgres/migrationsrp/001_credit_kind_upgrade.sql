-- Credit account product isolation for existing production databases.
-- Run AFTER the baseline schema is present and before enabling monthly credit.
-- Never merge exposures: each (customer, currency, credit_kind) is independent.
ALTER TABLE credit_accounts
    ADD COLUMN IF NOT EXISTS credit_kind text NOT NULL DEFAULT 'pay_in_4';

-- Existing rows represent the original installment product.
UPDATE credit_accounts SET credit_kind = 'pay_in_4' WHERE credit_kind IS NULL;

ALTER TABLE credit_accounts
    DROP CONSTRAINT IF EXISTS uq_credit_accounts_customer_currency;

ALTER TABLE credit_accounts
    ADD CONSTRAINT uq_credit_accounts_customer_currency_kind
        UNIQUE (customer_id, currency, credit_kind);

ALTER TABLE credit_accounts
    ADD CONSTRAINT chk_credit_accounts_credit_kind
        CHECK (credit_kind IN ('pay_in_4', 'pay_next_month'));
