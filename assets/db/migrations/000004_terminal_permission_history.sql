-- Retain revoked assignments; re-grant inserts a new historical assignment.
ALTER TABLE bnpl.merchant_terminal_permissions
    ADD COLUMN revoked_at timestamptz;

ALTER TABLE bnpl.merchant_terminal_permissions
    DROP CONSTRAINT uq_merchant_terminal_permissions_terminal_permission;

CREATE UNIQUE INDEX uq_merchant_terminal_permissions_active ON bnpl.merchant_terminal_permissions (merchant_terminal_id, permission)
WHERE
    revoked_at IS NULL;

COMMENT ON COLUMN bnpl.merchant_terminal_permissions.revoked_at IS 'Revocation evidence; NULL is active. Regrant creates a new assignment.';
