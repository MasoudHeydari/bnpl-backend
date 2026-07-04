-- ============================================================
-- 1. merchant_profiles (2 records)
-- ============================================================
INSERT INTO bnpl.merchant_profiles (id, external_merchant_id, display_name, legal_name, logo_url, support_phone, support_email, status, funding_models, settlement_models, currencies, risk_tier, max_amount_minor, max_amount_currency, max_tenure_months, requires_branch_mode, jurisdiction, created_at, updated_at)
VALUES
    ('dc520782-b4a1-4c3e-840a-8ce04f7bfacb', '1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc', 'Damz', 'Damz LLC', '', '', '', 'active', '["platform_funded"]', '["direct"]', '["IRR"]', 'low', 2000000000, 'IRR', 4, FALSE, 'IR', '2026-07-04 16:00:28.257+00', '2026-07-04 16:00:28.973+00'),
    ('2813b97e-086f-47d2-b9c0-48d74e32a9c7', '27afba8d-9fa8-400d-853a-fdf35619ad3a', 'Digikala', 'Digikala LLC', '', '', '', 'active', '["platform_funded"]', '["direct"]', '["IRR"]', 'low', 1000000000, 'IRR', 4, FALSE, 'IR', '2026-07-04 16:00:28.257+00', '2026-07-04 16:00:28.973+00');

-- ============================================================
-- 2. merchant_branch_profiles (4 records)
-- ============================================================
-- Two physical selling points plus one online outlet for the omnichannel demo merchant and one online-only merchant outlet.
INSERT INTO bnpl.merchant_branch_profiles (id, external_merchant_id, external_branch_id, branch_type, status, funding_models, settlement_models, currencies, risk_tier_override, max_amount_override_minor, max_amount_override_currency, max_tenure_months_override, jurisdiction, created_at, updated_at)
VALUES
    ('7c256ca9-c12e-480b-881f-76fa7c607b60', '1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc', 'f316f4d5-a614-4a54-bf1d-4441e5c9a98b', 'physical', 'active', '["platform_funded"]', '["direct"]', '["IRR"]', 'low', NULL, NULL, NULL, 'IR', '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00'),
    ('1c3d02e9-31d7-44d7-8bac-7da32a31544d', '1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc', 'faae36de-f9f7-41c0-be2c-cecae62363e4', 'physical', 'active', '["platform_funded"]', '["direct"]', '["IRR"]', 'low', NULL, NULL, NULL, 'IR', '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00'),
    ('0e19a753-9fa5-4a28-be01-edfeb2714696', '1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc', 'acbb7f04-64d9-4c91-848f-07c6e33423d6', 'online', 'active', '["platform_funded"]', '["direct"]', '["IRR"]', 'low', NULL, NULL, NULL, 'IR', '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00'),
    ('99ad39be-deb9-47fb-a105-dc1e1a99f302', '27afba8d-9fa8-400d-853a-fdf35619ad3a', 'ededbdfd-8da6-4990-a6ce-f0ba821485e8', 'online', 'active', '["platform_funded"]', '["direct"]', '["IRR"]', 'low', NULL, NULL, NULL, 'IR', '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00');

-- ============================================================
-- 3. merchant_pos_terminals (2 records)
-- ============================================================
-- POS/GPS identifiers are mapped independently so one physical selling point may own multiple terminals.
INSERT INTO bnpl.merchant_pos_terminals (id, merchant_branch_profile_id, external_pos_gps_id, enabled, created_at, updated_at)
VALUES
    ('711ab83b-0016-4fae-a3c5-63ad7c6c07c2', '7c256ca9-c12e-480b-881f-76fa7c607b60', '6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c', TRUE, '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00'),
    ('354dd0ce-93f1-4fbd-88c5-8c817630ad31', '1c3d02e9-31d7-44d7-8bac-7da32a31544d', '8517852d-0794-4ac6-b9eb-27ab333e91dc', TRUE, '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00');

-- ============================================================
-- 4. merchant_online_stores (2 records)
-- ============================================================
-- Online-store identifiers map virtual storefronts to online selling points without requiring a physical branch.
INSERT INTO bnpl.merchant_online_stores (id, merchant_branch_profile_id, external_online_store_id, enabled, created_at, updated_at)
VALUES
    ('fc09c1a4-169d-4347-9ff3-75ec8f9bc92a', '0e19a753-9fa5-4a28-be01-edfeb2714696', '8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c', TRUE, '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00'),
    ('454dd0ce-93f1-4fbd-88c5-8c817630ad33', '99ad39be-deb9-47fb-a105-dc1e1a99f302', '9e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c', TRUE, '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00');

-- ============================================================
-- 5. merchant_return_destinations (1 record)
-- ============================================================
-- Pre-registered return URL for Demo Merchant partner checkout handoff.
-- external_merchant_id must reference merchant_profiles.external_merchant_id (not profiles.id).
INSERT INTO bnpl.merchant_return_destinations (id, external_merchant_id, destination_name, return_url, enabled, created_at, updated_at)
    VALUES ('554dd0ce-93f1-4fbd-88c5-8c817630ad32', '1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc', 'default', 'http://localhost:3000/bnpl/return', TRUE, '2026-07-04 12:35:28.031501+00', '2026-07-04 12:35:28.031501+00');

-- -- ============================================================
-- -- 6. policy_assignment_imports (1 record)
-- -- ============================================================
-- INSERT INTO bnpl.policy_assignment_imports (id, import_type, source_file, status, total_count, success_count, failure_count, created_by, created_at, completed_at)
--     VALUES ('a3dbcc4e-defe-47f2-91a8-ef6a77e8381c', 'trusted_customers', 'trusted-subjects.csv', 'completed', 1, 1, 0, 'dev.masoud1997@gmail.com', '2026-09-22 15:53:26.77377+00', '2026-09-22 15:53:26.804507+00');
--
-- -- ============================================================
-- -- 7. policy_assignment_import_items (1 record)
-- -- ============================================================
-- INSERT INTO bnpl.policy_assignment_import_items (id, import_id, row_number, subject_id, mobile_hash, status, error_code, error_message, created_at, updated_at)
--     VALUES ('4473bef2-e3bd-42b7-83a8-19f5aa93805c', 'a3dbcc4e-defe-47f2-91a8-ef6a77e8381c', 2, '3ee847e6-2654-4537-931b-99ece376a35e', 'hmac-sha256:v1:0000000000000000000000000000000000000000000000000000000000000000', 'succeeded', NULL, NULL, '2026-09-22 15:53:26.77377+00', '2026-09-22 15:53:26.794102+00');
--
-- -- ============================================================
-- -- 8. policy_assignments (2 records)
-- -- ============================================================
-- INSERT INTO bnpl.policy_assignments (id, subject_type, subject_id, policy_family, policy_id, policy_version, reason, source, valid_from, valid_until, created_by, created_at, updated_at)
-- VALUES
--     ('6a12f533-23ef-4fa6-ae4e-30d58489ce89', 'customer', '3ee847e6-2654-4537-931b-99ece376a35e', 'checkout', 'checkout_contract_approval', 'v2', 'trusted_customer', 'csv_import', '2026-09-22 15:53:26.794102+00', NULL, 'dev.masoud1997@gmail.com', '2026-09-22 15:53:26.794102+00', '2026-09-22 15:53:26.794102+00'),
--     ('829a7f54-dfd6-4177-8d9a-110e2c9b83f6', 'customer', '3ee847e6-2654-4537-931b-99ece376a35e', 'initial_credit_limit', 'customer_default_initial_limit', 'v2', 'trusted_customer', 'csv_import', '2026-09-22 15:53:26.794102+00', NULL, 'dev.masoud1997@gmail.com', '2026-09-22 15:53:26.794102+00', '2026-09-22 15:53:26.794102+00');
