-- Specialized communication semantics only; general decisions are untouched.
ALTER TABLE bnpl.repayment_reminders
    DROP CONSTRAINT repayment_reminders_reminder_type_check;

ALTER TABLE bnpl.repayment_reminders
    ADD CONSTRAINT repayment_reminders_reminder_type_check CHECK (reminder_type IN ('pre_due', 'due_today', 'grace_period', 'grace_final_day', 'collections_default'));
