-- Demo data for local development. Never applied in production: it is run by
-- `make seed`, not by goose.
--
-- The three loans are chosen to exercise the reliability grading rather than to
-- look tidy: one confirmed and clean, one anchored on an unconfirmed balance,
-- one carrying arrears the engine refuses to plan around.

BEGIN;

INSERT INTO users (id, locale, timezone, trial_ends_at, access_state) VALUES
  ('11111111-1111-4111-8111-111111111111', 'hy', 'Asia/Yerevan', now() + interval '60 days', 'trial'),
  ('22222222-2222-4222-8222-222222222222', 'en', 'Asia/Yerevan', now() - interval '2 days',  'grace')
ON CONFLICT DO NOTHING;

INSERT INTO identities (user_id, telegram_user_enc, telegram_user_hmac, telegram_chat_enc, telegram_chat_hmac, key_version) VALUES
  ('11111111-1111-4111-8111-111111111111', '\x00', 'hmac-demo-user-1', '\x00', 'hmac-demo-chat-1', 1),
  ('22222222-2222-4222-8222-222222222222', '\x00', 'hmac-demo-user-2', '\x00', 'hmac-demo-chat-2', 1)
ON CONFLICT DO NOTHING;

INSERT INTO allocation_policy_versions
  (id, policy_key, version, definition, definition_schema_version, excess_rule, source_reference) VALUES
  ('aaaaaaa1-0000-4000-8000-000000000001', 'demo-consumer', 1,
   '{"order":["penalties","overdue_fees","unpaid_interest","current_fees","accrued_interest","overdue_principal","principal"]}',
   1, 'reduce_principal', 'synthetic fixture, not a real contract')
ON CONFLICT DO NOTHING;

INSERT INTO loans (id, user_id, name, lender, currency) VALUES
  ('10000000-0000-4000-8000-000000000001', '11111111-1111-4111-8111-111111111111', 'Consumer loan', 'Demo Bank', 'AMD'),
  ('10000000-0000-4000-8000-000000000002', '11111111-1111-4111-8111-111111111111', 'Card balance',  'Demo Bank', 'AMD'),
  ('10000000-0000-4000-8000-000000000003', '22222222-2222-4222-8222-222222222222', 'Car loan (USD)', 'Demo Bank', 'USD')
ON CONFLICT DO NOTHING;

INSERT INTO loan_contract_versions
  (id, loan_id, version, effective_from, nominal_rate, day_count, repayment_type,
   start_date, maturity_date, payment_day, scheduled_payment_minor, rounding_mode,
   rounding_unit_minor, allocation_policy_version_id, prepayment_policy, prepayment_schema_version) VALUES
  ('c0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000001', 1,
   '2024-01-05', 0.180000000, 'act365', 'annuity', '2024-01-05', '2029-01-05', 5,
   7450000, 'half_up', 100, 'aaaaaaa1-0000-4000-8000-000000000001', '{}', 1),
  ('c0000000-0000-4000-8000-000000000002', '10000000-0000-4000-8000-000000000002', 1,
   '2025-06-10', 0.240000000, 'act365', 'annuity', '2025-06-10', '2028-06-10', 10,
   3000000, 'half_up', 100, 'aaaaaaa1-0000-4000-8000-000000000001', '{}', 1),
  ('c0000000-0000-4000-8000-000000000003', '10000000-0000-4000-8000-000000000003', 1,
   '2025-03-01', 0.120000000, 'act360', 'annuity', '2025-03-01', '2030-03-01', 1,
   45000, 'half_up', 1, 'aaaaaaa1-0000-4000-8000-000000000001', '{}', 1)
ON CONFLICT DO NOTHING;

-- Loan 1: confirmed and clean.
INSERT INTO loan_snapshots
  (id, loan_id, contract_version_id, as_of, trust, principal_minor, accrued_interest_minor,
   next_installment_minor, next_due_date, remaining_installments, source_note, idempotency_key) VALUES
  ('50000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000001',
   'c0000000-0000-4000-8000-000000000001', current_date - 12, 'bank_confirmed',
   184000000, 0, 7450000, current_date + 18, 34, 'read from the bank app', 'seed-snap-1'),
-- Loan 2: the borrower typed a figure but never confirmed it.
  ('50000000-0000-4000-8000-000000000002', '10000000-0000-4000-8000-000000000002',
   'c0000000-0000-4000-8000-000000000002', current_date - 5, 'user_entered',
   50000000, 0, 3000000, current_date + 25, 20, 'typed from memory', 'seed-snap-2'),
-- Loan 3: arrears. Reminders continue, projections stop.
  ('50000000-0000-4000-8000-000000000003', '10000000-0000-4000-8000-000000000003',
   'c0000000-0000-4000-8000-000000000003', current_date - 40, 'bank_confirmed',
   1250000, 3200, 45000, current_date - 9, 55, 'statement', 'seed-snap-3')
ON CONFLICT DO NOTHING;

UPDATE loan_snapshots SET penalties_minor = 1500, overdue_principal_minor = 45000
 WHERE id = '50000000-0000-4000-8000-000000000003';

INSERT INTO loan_events
  (id, loan_id, contract_version_id, recorded_seq, kind, value_date, amount_minor,
   bank_reference, idempotency_key, fact_schema_version) VALUES
  ('e0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000001',
   'c0000000-0000-4000-8000-000000000001', 1, 'payment_reported', current_date - 7, 7450000,
   'TRX-8841', 'seed-ev-1', 1),
  ('e0000000-0000-4000-8000-000000000002', '10000000-0000-4000-8000-000000000001',
   'c0000000-0000-4000-8000-000000000001', 2, 'prepayment_reported', current_date - 3, 3000000,
   'TRX-8899', 'seed-ev-2', 1),
  ('e0000000-0000-4000-8000-000000000003', '10000000-0000-4000-8000-000000000002',
   'c0000000-0000-4000-8000-000000000002', 1, 'bank_fee_reported', current_date - 2, 50000,
   'FEE-114', 'seed-ev-3', 1)
ON CONFLICT DO NOTHING;

UPDATE loans SET next_event_seq = 3 WHERE id = '10000000-0000-4000-8000-000000000001';
UPDATE loans SET next_event_seq = 2 WHERE id = '10000000-0000-4000-8000-000000000002';

INSERT INTO loan_state
  (loan_id, state_version, anchor_snapshot_id, replay_generation, event_set_hash,
   last_recorded_seq, balance_as_of, principal_minor, accrued_interest_minor,
   reliability_state, engine_version) VALUES
  ('10000000-0000-4000-8000-000000000001', 3, '50000000-0000-4000-8000-000000000001',
   gen_random_uuid(), '\x00', 2, current_date, 174092700, 621400, 'confirmed', 'seed'),
  ('10000000-0000-4000-8000-000000000002', 1, '50000000-0000-4000-8000-000000000002',
   gen_random_uuid(), '\x00', 1, current_date, 50000000, 164400, 'estimated', 'seed'),
  ('10000000-0000-4000-8000-000000000003', 1, '50000000-0000-4000-8000-000000000003',
   gen_random_uuid(), '\x00', 0, current_date, 1250000, 17300, 'unsupported', 'seed')
ON CONFLICT (loan_id) DO NOTHING;

UPDATE loan_state SET penalties_minor = 1500, overdue_principal_minor = 45000,
       reliability_reasons = '[{"Code":"arrears_present","Detail":"penalties and overdue principal are outstanding"}]'
 WHERE loan_id = '10000000-0000-4000-8000-000000000003';

INSERT INTO telegram_commands
  (id, telegram_update_id, user_id, command_kind, payload_schema_version, status, completed_at) VALUES
  ('d0000000-0000-4000-8000-000000000001', 900001, '11111111-1111-4111-8111-111111111111',
   'record_payment', 1, 'completed', now() - interval '3 hours'),
  ('d0000000-0000-4000-8000-000000000002', 900002, '11111111-1111-4111-8111-111111111111',
   'add_loan', 1, 'pending', NULL)
ON CONFLICT DO NOTHING;

INSERT INTO reminder_occurrences
  (id, user_id, loan_id, due_date, offset_days, target_send_at, idempotency_key) VALUES
  ('60000000-0000-4000-8000-000000000001', '11111111-1111-4111-8111-111111111111',
   '10000000-0000-4000-8000-000000000001', current_date + 18, -3,
   (current_date + 15) + time '10:00', 'seed-occ-1')
ON CONFLICT DO NOTHING;

INSERT INTO notification_deliveries
  (id, user_id, delivery_kind, scheduled_at, group_key, payload, payload_schema_version,
   status, next_attempt_at, telegram_message_id, sent_at) VALUES
  ('70000000-0000-4000-8000-000000000001', '11111111-1111-4111-8111-111111111111',
   'reminder', now() - interval '1 day', 'seed-group-1', '{}', 1, 'sent',
   now() - interval '1 day', 55123, now() - interval '1 day'),
  ('70000000-0000-4000-8000-000000000002', '11111111-1111-4111-8111-111111111111',
   'reminder', now() + interval '3 days', 'seed-group-2', '{}', 1, 'pending',
   now() + interval '3 days', NULL, NULL)
ON CONFLICT DO NOTHING;

INSERT INTO reconciliation_runs
  (id, loan_id, new_snapshot_id, principal_drift_minor, interest_drift_minor,
   fee_drift_minor, penalty_drift_minor, engine_version) VALUES
  ('80000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000001',
   '50000000-0000-4000-8000-000000000001', -200, 1300, 0, 0, 'seed')
ON CONFLICT DO NOTHING;

COMMIT;
