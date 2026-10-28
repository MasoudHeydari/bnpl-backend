-- Permit independent restriction reasons without deleting existing history.
ALTER TABLE bnpl.customer_restrictions
    DROP CONSTRAINT customer_restrictions_reason_check;

ALTER TABLE bnpl.customer_restrictions
    ADD CONSTRAINT customer_restrictions_reason_check CHECK (reason ~ '^[a-z][a-z0-9_]{0,63}$');

DROP INDEX bnpl.customer_restrictions_one_active_type_idx;

CREATE UNIQUE INDEX customer_restrictions_one_active_reason_idx ON bnpl.customer_restrictions (customer_id, restriction_type, reason)
WHERE
    status = 'active';
