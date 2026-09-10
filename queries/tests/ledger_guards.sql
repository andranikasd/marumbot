-- Run only against a disposable database, as its owner. Everything rolls back.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO users(id,trial_ends_at) VALUES('00000000-0000-0000-0000-000000000024',now());
INSERT INTO loans(id,user_id,name,currency) VALUES('00000000-0000-0000-0000-000000000024','00000000-0000-0000-0000-000000000024','guard fixture','AMD');
INSERT INTO billing_events(id,user_id,provider,kind,external_id,occurred_at)
VALUES('00000000-0000-0000-0000-000000000024','00000000-0000-0000-0000-000000000024','fixture','manual_grant','ledger-guard-fixture',now());
INSERT INTO loan_contract_versions(id,loan_id,version,effective_from,nominal_rate,day_count,repayment_type,start_date,maturity_date,payment_day,rounding_mode,rounding_unit_minor,allocation_policy_version_id,prepayment_schema_version)
SELECT '00000000-0000-0000-0000-000000000024','00000000-0000-0000-0000-000000000024',1,'2026-01-01',0,'act365','annuity','2026-01-01','2027-01-01',1,'half_up',1,id,1 FROM allocation_policy_versions LIMIT 1;
INSERT INTO loan_events(id,loan_id,contract_version_id,recorded_seq,kind,value_date,idempotency_key,fact_schema_version)
VALUES('00000000-0000-0000-0000-000000000024','00000000-0000-0000-0000-000000000024','00000000-0000-0000-0000-000000000024',1,'payment_reported','2026-01-01','ledger-guard-fixture',1);
DO $$ BEGIN
 BEGIN UPDATE loan_events SET amount_minor=1 WHERE idempotency_key='ledger-guard-fixture'; RAISE EXCEPTION 'loan UPDATE guard missing'; EXCEPTION WHEN SQLSTATE '55000' THEN NULL; END;
 BEGIN DELETE FROM loan_events WHERE idempotency_key='ledger-guard-fixture'; RAISE EXCEPTION 'loan DELETE guard missing'; EXCEPTION WHEN SQLSTATE '55000' THEN NULL; END;
 BEGIN UPDATE billing_events SET amount_minor=1 WHERE external_id='ledger-guard-fixture'; RAISE EXCEPTION 'UPDATE guard missing'; EXCEPTION WHEN SQLSTATE '55000' THEN NULL; END;
 BEGIN DELETE FROM billing_events WHERE external_id='ledger-guard-fixture'; RAISE EXCEPTION 'DELETE guard missing'; EXCEPTION WHEN SQLSTATE '55000' THEN NULL; END;
 BEGIN TRUNCATE loan_events CASCADE; RAISE EXCEPTION 'TRUNCATE guard missing'; EXCEPTION WHEN SQLSTATE '55000' THEN NULL; END;
 BEGIN DELETE FROM loans WHERE id='00000000-0000-0000-0000-000000000024'; RAISE EXCEPTION 'loan deletion guard missing'; EXCEPTION WHEN SQLSTATE '55000' THEN NULL; END;
 BEGIN DELETE FROM users WHERE id='00000000-0000-0000-0000-000000000024'; RAISE EXCEPTION 'erasure request guard missing'; EXCEPTION WHEN SQLSTATE '55000' THEN NULL; END;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='marum_app') THEN
 IF has_table_privilege('marum_app','loan_events','UPDATE') OR has_table_privilege('marum_app','billing_events','DELETE') THEN RAISE EXCEPTION 'application can mutate ledger'; END IF;
 END IF;
END $$;
UPDATE users SET deletion_requested_at=now() WHERE id='00000000-0000-0000-0000-000000000024';
DELETE FROM users WHERE id='00000000-0000-0000-0000-000000000024';
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM loans WHERE id='00000000-0000-0000-0000-000000000024') OR EXISTS(SELECT 1 FROM billing_events WHERE external_id='ledger-guard-fixture') THEN RAISE EXCEPTION 'requested erasure did not cascade'; END IF;
END $$;
ROLLBACK;
