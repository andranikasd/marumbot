-- Run after an ACL-free restore, as the database owner.
REVOKE UPDATE, DELETE, TRUNCATE ON loan_events, billing_events FROM marum_app;
REVOKE DELETE, TRUNCATE ON loans FROM marum_app;
