-- Application ordering must be deployed with writers drained; see database-invariants.md.
-- No financial history is repaired by this migration.
LOCK TABLE bnpl.installments,
bnpl.installment_charges,
bnpl.payment_attempts,
bnpl.payment_attempt_items IN SHARE ROW EXCLUSIVE MODE;

DO $preflight$
DECLARE
    bad record;
BEGIN
    SELECT
        i.customer_id,
        i.contract_id,
        i.id AS installment_id,
        c.id AS charge_id
    INTO
        bad
    FROM
        bnpl.installments i
        JOIN bnpl.installment_charges c ON c.installment_id = i.id
    WHERE
        c.payer_type = 'customer'
        AND ((i.status = 'paid'
                AND c.charge_status IN ('offered', 'applied'))
            OR (i.status <> 'paid'
                AND (c.charge_status = 'paid'
                    OR (c.charge_type IN ('discount', 'waiver')
                        AND c.charge_status = 'waived'))))
    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN002', CONSTRAINT = 'installment_has_unsettled_charges', MESSAGE = format('Preflight failed: customer=%s contract=%s installment=%s charge=%s. Run assets/db/audits/installment_integrity.sql; reconcile explicitly before migrating.', bad.customer_id, bad.contract_id, bad.installment_id, bad.charge_id);
    END IF;
    SELECT
        i.customer_id,
        i.contract_id,
        i.id AS installment_id,
        p.id AS attempt_id
    INTO
        bad
    FROM
        bnpl.payment_attempts p
        JOIN bnpl.installments i ON i.id = p.installment_id
    WHERE
        p.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required')
        AND (p.currency <> i.currency
            OR p.contract_id IS DISTINCT FROM i.contract_id
            OR NOT EXISTS (
                SELECT
                    1
                FROM
                    bnpl.payment_attempt_items x
                WHERE
                    x.payment_attempt_id = p.id
                    AND x.item_type = 'principal'
                    AND x.source_id = i.id
                    AND x.amount_minor = i.principal_amount_minor
                    AND x.effect = 'debit')
                OR (i.upfront_amount_minor > 0
                    AND NOT EXISTS (
                        SELECT
                            1
                        FROM
                            bnpl.payment_attempt_items x
                        WHERE
                            x.payment_attempt_id = p.id
                            AND x.item_type = 'upfront'
                            AND x.source_id = i.id
                            AND x.amount_minor = i.upfront_amount_minor
                            AND x.effect = 'debit'))
                    OR p.amount_minor <> (
                        SELECT
                            coalesce(sum(
                                    CASE WHEN x.effect = 'debit' THEN
                                        x.amount_minor
                                    ELSE
                                        - x.amount_minor
                                    END), 0)
                        FROM
                            bnpl.payment_attempt_items x
                        WHERE
                            x.payment_attempt_id = p.id)
                        OR EXISTS (
                            SELECT
                                1
                            FROM
                                bnpl.installment_charges c
                            WHERE
                                c.installment_id = i.id
                                AND c.payer_type = 'customer'
                                AND c.charge_status IN ('offered', 'applied')
                                AND NOT EXISTS (
                                    SELECT
                                        1
                                    FROM
                                        bnpl.payment_attempt_items x
                                    WHERE
                                        x.payment_attempt_id = p.id
                                        AND x.item_type = 'charge'
                                        AND x.source_id = c.id
                                        AND x.amount_minor = c.amount_minor)))
                    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = format('Preflight failed: customer=%s contract=%s installment=%s attempt=%s has an inconsistent unresolved snapshot. Audit and reconcile explicitly before migration.', bad.customer_id, bad.contract_id, bad.installment_id, bad.attempt_id);
    END IF;
END
$preflight$;

ALTER TABLE bnpl.installments
    ADD COLUMN financial_revision bigint NOT NULL DEFAULT 0 CHECK (financial_revision >= 0);

ALTER TABLE bnpl.payment_attempts
    ADD COLUMN financial_outcome_resolved boolean NOT NULL DEFAULT FALSE;

ALTER TABLE bnpl.payment_attempts
    ADD COLUMN snapshot_xid xid8 NOT NULL DEFAULT pg_current_xact_id ();

CREATE FUNCTION bnpl.guard_installment_charge ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    SET search_path = pg_catalog, bnpl
    AS $body$
DECLARE
    parent bnpl.installments%ROWTYPE;
    frozen boolean;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Financial charge history cannot be deleted';
    END IF;
    IF TG_OP = 'UPDATE' AND NEW.installment_id IS DISTINCT FROM OLD.installment_id THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Charge ownership is immutable';
    END IF;
    -- NOWAIT avoids the child-row -> parent-row deadlock of arbitrary UPDATE SQL.
    BEGIN
        SELECT
            *
        INTO
            parent
        FROM
            bnpl.installments
        WHERE
            id = NEW.installment_id
        FOR UPDATE
            NOWAIT;
    EXCEPTION
        WHEN lock_not_available THEN
            RAISE EXCEPTION USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Installment financial operation is in progress';
    END;
    IF parent.status = 'paid' THEN
        -- Only an exact no-op is permitted; even metadata is frozen after settlement.
        IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
            RAISE EXCEPTION USING ERRCODE = 'BN001', CONSTRAINT = 'installment_already_paid', MESSAGE = 'Settled installment is immutable';
            END IF;
            RETURN NEW;
        END IF;
        SELECT
            EXISTS (
                SELECT
                    1
                FROM
                    bnpl.payment_attempts p
                WHERE
                    p.installment_id = NEW.installment_id
                    AND (p.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required', 'succeeded')
                        OR (p.state IN ('failed', 'canceled')
                            AND NOT p.financial_outcome_resolved)))
        INTO
            frozen;
            UPDATE
                bnpl.installments
            SET
                financial_revision = financial_revision + 1
            WHERE
                id = NEW.installment_id;
                IF frozen THEN
                    IF TG_OP = 'UPDATE' AND OLD.payer_type <> 'customer' AND OLD.charge_status = 'offered' AND NEW.charge_status = 'applied' AND (to_jsonb (NEW) - ARRAY['charge_status', 'updated_at']) = (to_jsonb (OLD) - ARRAY['charge_status', 'updated_at']) THEN
                        RETURN NEW;
                        END IF;
                        IF TG_OP = 'UPDATE' AND (to_jsonb (NEW) - ARRAY['charge_status', 'paid_at', 'updated_at']) = (to_jsonb (OLD) - ARRAY['charge_status', 'paid_at', 'updated_at']) AND OLD.charge_status IN ('offered', 'applied') AND ((OLD.charge_type IN ('discount', 'waiver') AND NEW.charge_status = 'waived' AND NEW.paid_at IS NULL) OR (OLD.charge_type NOT IN ('discount', 'waiver') AND NEW.charge_status = 'paid' AND NEW.paid_at IS NOT NULL)) AND OLD.payer_type = 'customer' AND EXISTS (
        SELECT
            1
        FROM
            bnpl.payment_attempts p
            JOIN bnpl.payment_attempt_items x ON x.payment_attempt_id = p.id
        WHERE
            p.installment_id = parent.id AND p.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required', 'succeeded') AND x.item_type = 'charge' AND x.source_id = OLD.id AND x.amount_minor = OLD.amount_minor AND x.effect = CASE WHEN OLD.charge_type IN ('discount', 'waiver') THEN
                'credit'
            ELSE
                'debit'
            END) THEN
    RETURN NEW;
    -- deferred guard requires complete successful settlement at commit
                            END IF;
                            IF TG_OP = 'UPDATE' AND NEW IS NOT DISTINCT FROM OLD THEN
                                RETURN NEW;
                                END IF;
                                RAISE EXCEPTION USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Payment snapshot freezes financial components';
                    END IF;
                    RETURN NEW;
END
$body$;

CREATE FUNCTION bnpl.guard_installment_settlement ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    SET search_path = pg_catalog, bnpl
    AS $body$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Installment history cannot be deleted';
    END IF;
    IF OLD.status = 'paid' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN001', CONSTRAINT = 'installment_already_paid', MESSAGE = 'Paid installment is terminal and immutable';
    END IF;
    IF (NEW.id,
        NEW.merchant_profile_id,
        NEW.merchant_branch_profile_id,
        NEW.principal_amount_minor,
        NEW.upfront_amount_minor,
        NEW.currency,
        NEW.contract_id,
        NEW.customer_id,
        NEW.decision_id,
        NEW.installment_order,
        NEW.total_amount_minor,
        NEW.base_fee_amount_minor,
        NEW.due_at,
        NEW.grace_period_days) IS DISTINCT FROM (OLD.id,
    OLD.merchant_profile_id,
    OLD.merchant_branch_profile_id,
    OLD.principal_amount_minor,
    OLD.upfront_amount_minor,
    OLD.currency,
    OLD.contract_id,
    OLD.customer_id,
    OLD.decision_id,
    OLD.installment_order,
    OLD.total_amount_minor,
    OLD.base_fee_amount_minor,
    OLD.due_at,
    OLD.grace_period_days) AND EXISTS (
SELECT
    1
FROM
    bnpl.payment_attempts p
WHERE
    p.installment_id = OLD.id AND (p.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required', 'succeeded') OR (p.state IN ('failed', 'canceled') AND NOT p.financial_outcome_resolved))) THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Payment snapshot freezes installment economics';
    END IF;
    IF TG_OP = 'UPDATE' AND NEW.status IS DISTINCT FROM OLD.status AND NEW.status NOT IN ('scheduled', 'due', 'overdue', 'paid') AND EXISTS (
        SELECT
            1
        FROM
            bnpl.payment_attempts p
        WHERE
            p.installment_id = OLD.id AND (p.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required', 'succeeded') OR (p.state IN ('failed', 'canceled') AND NOT p.financial_outcome_resolved))) THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Unresolved payment prevents extinguishing its installment';
    END IF;
    IF NEW.status = 'paid' AND EXISTS (
        SELECT
            1
        FROM
            bnpl.installment_charges c
        WHERE
            c.installment_id = NEW.id AND c.payer_type = 'customer' AND c.charge_status IN ('offered', 'applied')) THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN002', CONSTRAINT = 'installment_has_unsettled_charges', MESSAGE = 'Customer components must be finalized before installment settlement';
    END IF;
    IF NEW.status = 'paid' AND NOT EXISTS (
        SELECT
            1
        FROM
            bnpl.payment_attempts p
            JOIN bnpl.payment_attempt_items x ON x.payment_attempt_id = p.id
        WHERE
            p.installment_id = NEW.id AND p.amount_minor = NEW.paid_amount_minor AND p.currency = NEW.currency AND p.contract_id IS NOT DISTINCT FROM NEW.contract_id AND p.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required', 'succeeded') AND x.item_type = 'principal' AND x.source_id = NEW.id AND x.amount_minor = NEW.principal_amount_minor AND x.effect = 'debit') THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN002', CONSTRAINT = 'installment_has_unsettled_charges', MESSAGE = 'Full installment cash must match its immutable payment snapshot';
    END IF;
    RETURN NEW;
END
$body$;

CREATE FUNCTION bnpl.guard_payment_attempt_freeze ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    SET search_path = pg_catalog, bnpl
    AS $body$
DECLARE
    parent bnpl.installments%ROWTYPE;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Payment history cannot be deleted';
    END IF;
    IF TG_OP = 'UPDATE' AND (NEW.id,
        NEW.idempotency_key,
        NEW.request_fingerprint,
        NEW.created_at,
        NEW.installment_id,
        NEW.contract_id,
        NEW.amount_minor,
        NEW.currency,
        NEW.purpose,
        NEW.snapshot_xid) IS DISTINCT FROM (OLD.id,
    OLD.idempotency_key,
    OLD.request_fingerprint,
    OLD.created_at,
    OLD.installment_id,
    OLD.contract_id,
    OLD.amount_minor,
    OLD.currency,
    OLD.purpose,
    OLD.snapshot_xid) THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Payment economic identity is immutable';
    END IF;
    BEGIN
        SELECT
            *
        INTO
            parent
        FROM
            bnpl.installments
        WHERE
            id = NEW.installment_id
        FOR UPDATE
            NOWAIT;
    EXCEPTION
        WHEN lock_not_available THEN
            RAISE EXCEPTION USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Installment financial operation is in progress';
    END;
    IF TG_OP = 'UPDATE' AND ((OLD.financial_payment_intent_id IS NOT NULL AND NEW.financial_payment_intent_id IS DISTINCT FROM OLD.financial_payment_intent_id) OR (OLD.financial_collection_obligation_id IS NOT NULL AND NEW.financial_collection_obligation_id IS DISTINCT FROM OLD.financial_collection_obligation_id) OR (OLD.state = 'succeeded' AND (NEW.financial_paid_at,
NEW.applied_at) IS DISTINCT FROM (OLD.financial_paid_at,
OLD.applied_at))) THEN
        RAISE EXCEPTION USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Payment financial identity and effective application evidence are immutable';
        END IF;
        IF TG_OP = 'UPDATE' AND OLD.state = 'succeeded' AND NEW.state <> OLD.state THEN
            RAISE EXCEPTION USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Successful payment history is terminal';
            END IF;
            IF TG_OP = 'INSERT' AND EXISTS (
                SELECT
                    1
                FROM
                    bnpl.payment_attempts p
                WHERE
                    p.installment_id = NEW.installment_id AND (p.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required', 'succeeded') OR (p.state IN ('failed', 'canceled') AND NOT p.financial_outcome_resolved))) THEN
                RAISE EXCEPTION USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Existing financial outcome must be resolved before another payment';
                END IF;
                IF TG_OP = 'INSERT' AND parent.status = 'paid' THEN
                    RAISE EXCEPTION USING ERRCODE = 'BN001', CONSTRAINT = 'installment_already_paid', MESSAGE = 'Cannot pay a settled installment again';
                    END IF;
                    IF TG_OP = 'INSERT' THEN
                        NEW.snapshot_xid := pg_current_xact_id ();
                        UPDATE
                            bnpl.installments
                        SET
                            financial_revision = financial_revision + 1
                        WHERE
                            id = NEW.installment_id;
                        END IF;
                        RETURN NEW;
END
$body$;

CREATE FUNCTION bnpl.guard_payment_items ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    SET search_path = pg_catalog, bnpl
    AS $body$
DECLARE
    parent_id uuid;
    creator xid8;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Payment items are immutable history';
    END IF;
    SELECT
        installment_id,
        snapshot_xid
    INTO
        parent_id,
        creator
    FROM
        bnpl.payment_attempts
    WHERE
        id = NEW.payment_attempt_id;
    IF creator IS DISTINCT FROM pg_current_xact_id () THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Payment snapshot can only be created with its attempt';
    END IF;
    BEGIN
        PERFORM
            1
        FROM
            bnpl.installments
        WHERE
            id = parent_id
        FOR UPDATE
            NOWAIT;
    EXCEPTION
        WHEN lock_not_available THEN
            RAISE EXCEPTION USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Installment financial operation is in progress';
    END;
    RETURN NEW;
END
$body$;

-- Deferred validation permits ordered item inserts, and checkout's successful
-- attempt bookkeeping after charge application, but never an incomplete commit.
CREATE FUNCTION bnpl.verify_payment_snapshot_commit ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    SET search_path = pg_catalog, bnpl
    AS $body$
DECLARE
    pid uuid;
    p bnpl.payment_attempts%ROWTYPE;
    i bnpl.installments%ROWTYPE;
BEGIN
    IF TG_TABLE_NAME = 'installments' THEN
        IF NEW.status = 'paid' AND NOT EXISTS (
            SELECT
                1
            FROM
                bnpl.payment_attempts evidence
            WHERE
                evidence.installment_id = NEW.id AND evidence.state = 'succeeded' AND evidence.amount_minor = NEW.paid_amount_minor AND evidence.currency = NEW.currency) THEN
            RAISE EXCEPTION
                USING ERRCODE = 'BN002', CONSTRAINT = 'installment_has_unsettled_charges', MESSAGE = 'Full installment settlement requires successful payment evidence at commit';
        END IF;
        RETURN NULL;
    END IF;
    IF TG_TABLE_NAME = 'payment_attempts' THEN
        pid := NEW.id;
    ELSIF TG_TABLE_NAME = 'payment_attempt_items' THEN
        pid := NEW.payment_attempt_id;
    ELSE
        IF OLD.charge_status IS NOT DISTINCT FROM NEW.charge_status OR NEW.charge_status NOT IN ('paid', 'waived') THEN
            RETURN NULL;
        END IF;
        IF NEW.payer_type <> 'customer' OR (NEW.charge_status = 'waived' AND NEW.charge_type NOT IN ('discount', 'waiver')) THEN
            RETURN NULL;
        END IF;
        IF NOT EXISTS (
            SELECT
                1
            FROM
                bnpl.installments
            WHERE
                id = NEW.installment_id
                AND status = 'paid') THEN
        RAISE EXCEPTION
            USING ERRCODE = 'BN002', CONSTRAINT = 'installment_has_unsettled_charges', MESSAGE = 'Customer settlement cannot commit separately from full installment settlement';
    END IF;
    IF NOT EXISTS (
        SELECT
            1
        FROM
            bnpl.payment_attempts a
        WHERE
            a.installment_id = NEW.installment_id
            AND (a.state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required', 'succeeded')
                OR (a.state IN ('failed', 'canceled')
                    AND NOT a.financial_outcome_resolved))) THEN
    RETURN NULL;
END IF;
    IF NOT EXISTS (
        SELECT
            1
        FROM
            bnpl.installments
        WHERE
            id = NEW.installment_id
            AND status = 'paid')
    OR NOT EXISTS (
        SELECT
            1
        FROM
            bnpl.payment_attempts a
            JOIN bnpl.payment_attempt_items x ON x.payment_attempt_id = a.id
        WHERE
            a.installment_id = NEW.installment_id
            AND a.state = 'succeeded'
            AND x.item_type = 'charge'
            AND x.source_id = NEW.id
            AND x.amount_minor = NEW.amount_minor) THEN
    RAISE EXCEPTION
    USING ERRCODE = 'BN003', CONSTRAINT = 'installment_payment_in_progress', MESSAGE = 'Component settlement requires atomic successful installment application';
END IF;
    RETURN NULL;
END IF;
    SELECT
        *
    INTO
        p
    FROM
        bnpl.payment_attempts
    WHERE
        id = pid;
    SELECT
        *
    INTO
        i
    FROM
        bnpl.installments
    WHERE
        id = p.installment_id;
    IF p.id IS NULL THEN
        RETURN NULL;
    END IF;
    IF (
        SELECT
            count(*)
        FROM
            bnpl.payment_attempt_items x
        WHERE
            x.payment_attempt_id = pid AND x.item_type = 'principal' AND x.source_id = i.id AND x.amount_minor = i.principal_amount_minor AND x.effect = 'debit') <> 1 OR (
    SELECT
        count(*)
    FROM
        bnpl.payment_attempt_items x
    WHERE
        x.payment_attempt_id = pid AND x.item_type = 'upfront') <> (
CASE WHEN i.upfront_amount_minor > 0 THEN
    1
ELSE
    0
END)
    OR EXISTS (
        SELECT
            1
        FROM
            bnpl.payment_attempt_items x
        WHERE
            x.payment_attempt_id = pid
            AND ((x.item_type = 'principal'
                    AND (x.source_id <> i.id
                        OR x.amount_minor <> i.principal_amount_minor
                        OR x.effect <> 'debit'))
                OR (x.item_type = 'upfront'
                    AND (x.source_id <> i.id
                        OR x.amount_minor <> i.upfront_amount_minor
                        OR x.effect <> 'debit'))
                OR (x.item_type = 'charge'
                    AND NOT EXISTS (
                        SELECT
                            1
                        FROM
                            bnpl.installment_charges c
                        WHERE
                            c.id = x.source_id
                            AND c.installment_id = i.id
                            AND c.payer_type = 'customer'
                            AND c.amount_minor = x.amount_minor
                            AND c.currency = p.currency
                            AND x.effect = CASE WHEN c.charge_type IN ('discount', 'waiver') THEN
                                'credit'
                            ELSE
                                'debit'
                            END))
                    OR x.item_type NOT IN ('principal', 'upfront', 'charge')))
            OR EXISTS (
                SELECT
                    1
                FROM
                    bnpl.installment_charges c
                WHERE
                    c.installment_id = i.id
                    AND c.payer_type = 'customer'
                    AND c.charge_status IN ('offered', 'applied')
                    AND NOT EXISTS (
                        SELECT
                            1
                        FROM
                            bnpl.payment_attempt_items x
                        WHERE
                            x.payment_attempt_id = pid
                            AND x.item_type = 'charge'
                            AND x.source_id = c.id))
                OR EXISTS (
                    SELECT
                        1
                    FROM
                        bnpl.payment_attempt_items x
                    WHERE
                        x.payment_attempt_id = pid
                    GROUP BY
                        x.item_type,
                        x.source_id
                    HAVING
                        count(*) > 1)
                OR p.currency <> i.currency
                OR p.contract_id IS DISTINCT FROM i.contract_id
                OR p.amount_minor <> (
                    SELECT
                        coalesce(sum(
                                CASE WHEN x.effect = 'debit' THEN
                                    x.amount_minor
                                ELSE
                                    - x.amount_minor
                                END), 0)
                    FROM
                        bnpl.payment_attempt_items x
                    WHERE
                        x.payment_attempt_id = pid) THEN
                RAISE EXCEPTION
            USING ERRCODE = 'BN004', CONSTRAINT = 'installment_financial_component_immutable', MESSAGE = 'Payment snapshot does not cover the complete installment';
    END IF;
    RETURN NULL;
END
$body$;

CREATE TRIGGER installment_no_charges_after_paid
    BEFORE INSERT OR UPDATE OR DELETE ON bnpl.installment_charges
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.guard_installment_charge ();

CREATE TRIGGER installment_complete_settlement
    BEFORE INSERT OR UPDATE OR DELETE ON bnpl.installments
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.guard_installment_settlement ();

CREATE TRIGGER installment_payment_freeze
    BEFORE INSERT OR UPDATE OR DELETE ON bnpl.payment_attempts
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.guard_payment_attempt_freeze ();

CREATE TRIGGER installment_snapshot_immutable
    BEFORE INSERT OR UPDATE OR DELETE ON bnpl.payment_attempt_items
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.guard_payment_items ();

CREATE CONSTRAINT TRIGGER installment_snapshot_complete
    AFTER INSERT ON bnpl.payment_attempts DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.verify_payment_snapshot_commit ();

CREATE CONSTRAINT TRIGGER installment_items_complete
    AFTER INSERT ON bnpl.payment_attempt_items DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.verify_payment_snapshot_commit ();

CREATE CONSTRAINT TRIGGER installment_components_settled
    AFTER UPDATE ON bnpl.installment_charges DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.verify_payment_snapshot_commit ();

CREATE CONSTRAINT TRIGGER installment_paid_evidence
    AFTER INSERT OR UPDATE ON bnpl.installments DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION bnpl.verify_payment_snapshot_commit ();
