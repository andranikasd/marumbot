-- name: EnqueueLoanFiled
WITH rules AS (
 INSERT INTO reminder_rules(id,loan_id,offset_days,send_at_local)
 SELECT gen_random_uuid(),$1,v.offset_days,'10:00'::time FROM (VALUES(-3),(0))v(offset_days)
 ON CONFLICT(loan_id,offset_days) DO NOTHING
)
INSERT INTO notification_deliveries(id,user_id,delivery_kind,scheduled_at,group_key,payload,payload_schema_version,next_attempt_at)
SELECT gen_random_uuid(),user_id,'loan_filed',now(),'loan_filed:'||id::text,jsonb_build_object('loan_id',id),1,now()
FROM loans WHERE id=$1 ON CONFLICT(group_key) DO NOTHING;

-- name: LeaseLoanFiled
WITH due AS (
 SELECT n.id FROM notification_deliveries n JOIN users u ON u.id=n.user_id
 WHERE n.delivery_kind='loan_filed' AND u.deleted_at IS NULL AND u.access_state<>'paused'
 AND ((n.status='pending' AND n.next_attempt_at<=$1) OR (n.status='leased' AND n.lease_until<=$1))
 ORDER BY n.next_attempt_at,n.id LIMIT $2 FOR UPDATE OF n SKIP LOCKED
)
UPDATE notification_deliveries n SET status='leased',lease_token=gen_random_uuid(),lease_until=$1::timestamptz+interval '2 minutes',attempts=least(attempts+1,20)
FROM due WHERE n.id=due.id RETURNING n.id,n.user_id,n.payload->>'loan_id',n.lease_token;

-- name: CompleteLoanFiled
UPDATE notification_deliveries SET status='sent',sent_at=$3,lease_until=NULL,lease_token=NULL
WHERE id=$1 AND lease_token=$2 AND status='leased';

-- name: RetryLoanFiled
UPDATE notification_deliveries SET status='pending',next_attempt_at=$3::timestamptz+make_interval(secs=>least(21600,60*(1<<least(attempts-1,9)))),lease_until=NULL,lease_token=NULL,last_error_code='send_failed'
WHERE id=$1 AND lease_token=$2 AND status='leased';
