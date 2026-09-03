-- name: ScheduleOptionalReminder
-- One notification per approved action, on the action day at the loan's
-- existing on-day reminder time. No recomputable payment amount is stored.
INSERT INTO reminder_occurrences(id,user_id,loan_id,due_date,offset_days,target_send_at,
 idempotency_key,approved_plan_id,plan_action_index)
SELECT gen_random_uuid(),l.user_id,l.id,$5::date,0,
 ($5::date+r.send_at_local) AT TIME ZONE u.timezone,
 'optional:'||p.id::text||':'||$4::integer::text,p.id,$4::integer
FROM loans l JOIN users u ON u.id=l.user_id
JOIN plan_versions p ON p.user_id=l.user_id AND p.currency=l.currency
JOIN reminder_rules r ON r.loan_id=l.id AND r.offset_days=0 AND r.enabled
WHERE l.user_id=$1 AND p.id=$2 AND l.id=$3 AND l.archived_at IS NULL AND u.deleted_at IS NULL
AND p.id=(SELECT a.plan_id FROM plan_activation_events a JOIN plan_versions v ON v.id=a.plan_id
 WHERE a.user_id=$1 AND v.currency=p.currency ORDER BY a.revision DESC LIMIT 1)
ON CONFLICT DO NOTHING;

-- name: CancelObsoleteOptionalReminders
UPDATE reminder_occurrences SET status='canceled',preference_version=preference_version+1
WHERE user_id=$1 AND approved_plan_id IS NOT NULL AND status IN ('scheduled','attached')
AND NOT (approved_plan_id::text=ANY($2::text[]));

-- name: CancelOptionalReminder
UPDATE reminder_occurrences SET status='canceled',preference_version=preference_version+1
WHERE id=$1 AND approved_plan_id IS NOT NULL AND status='scheduled';

-- name: DueOptionalReminders
SELECT o.id,o.user_id,o.loan_id,o.due_date::text,o.offset_days,l.name,l.currency,
 o.approved_plan_id::text,o.plan_action_index
FROM reminder_occurrences o JOIN loans l ON l.id=o.loan_id JOIN users u ON u.id=o.user_id
CROSS JOIN LATERAL (SELECT extract(hour FROM ($1::timestamptz AT TIME ZONE u.timezone))*60+
 extract(minute FROM ($1::timestamptz AT TIME ZONE u.timezone)) AS minute) local_time
WHERE o.approved_plan_id IS NOT NULL AND o.status='scheduled' AND o.target_send_at<=$1
AND l.archived_at IS NULL AND u.deleted_at IS NULL AND u.access_state<>'paused'
AND (NOT u.quiet_enabled OR NOT CASE WHEN u.quiet_start<u.quiet_end
 THEN local_time.minute>=u.quiet_start AND local_time.minute<u.quiet_end
 ELSE local_time.minute>=u.quiet_start OR local_time.minute<u.quiet_end END)
ORDER BY o.target_send_at,o.id LIMIT $2;
