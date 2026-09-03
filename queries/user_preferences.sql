-- name: GetUserPreferences
SELECT timezone,quiet_enabled,quiet_start,quiet_end,settings_version
FROM users WHERE id=$1 AND deleted_at IS NULL;

-- name: LockPreferenceUser
SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE;

-- name: PreferenceReceipt
SELECT payload,result FROM user_preference_receipts WHERE user_id=$1 AND command_key=$2;

-- name: SavePreferenceReceipt
INSERT INTO user_preference_receipts(user_id,command_key,payload,result) VALUES($1,$2,$3,$4);

-- name: UpdateUserPreferences
UPDATE users SET timezone=$2,quiet_enabled=$3,quiet_start=$4,quiet_end=$5,settings_version=settings_version+1
WHERE id=$1 AND settings_version=$6 AND deleted_at IS NULL
RETURNING timezone,quiet_enabled,quiet_start,quiet_end,settings_version;

-- name: RetimeUserReminders
-- Explicit snoozes are absolute instants. Only unsnoozed scheduled reminders
-- follow a timezone edit; neither kind changes its contract due date.
UPDATE reminder_occurrences o SET target_send_at=((o.due_date+o.offset_days*interval '1 day')+r.send_at_local) AT TIME ZONE $2,
 preference_version=preference_version+1
FROM reminder_rules r WHERE o.user_id=$1 AND o.status='scheduled' AND NOT o.snoozed
AND r.loan_id=o.loan_id AND r.offset_days=o.offset_days;

-- name: PreferenceOccurrence
SELECT o.id,o.loan_id,o.due_date::text,o.target_send_at,o.status,o.preference_version,(o.approved_plan_id IS NULL)
FROM reminder_occurrences o JOIN loans l ON l.id=o.loan_id
JOIN users u ON u.id=o.user_id
WHERE o.user_id=$1 AND o.id=$2 AND l.archived_at IS NULL AND u.deleted_at IS NULL;

-- name: SnoozePreferenceOccurrence
UPDATE reminder_occurrences o SET target_send_at=$3,status='scheduled',snoozed=true,preference_version=preference_version+1
WHERE o.user_id=$1 AND o.id=$2 AND o.preference_version=$4 AND o.status IN ('scheduled','satisfied')
AND EXISTS(SELECT 1 FROM loans l WHERE l.id=o.loan_id AND l.user_id=$1 AND l.archived_at IS NULL)
RETURNING o.id,o.loan_id,o.due_date::text,o.target_send_at,o.status,o.preference_version,(o.approved_plan_id IS NULL);

-- name: ReadyPreferenceReminders
SELECT o.id,o.user_id,o.loan_id,o.due_date::text,o.offset_days,l.name,l.currency
FROM reminder_occurrences o JOIN loans l ON l.id=o.loan_id JOIN users u ON u.id=o.user_id
CROSS JOIN LATERAL (SELECT extract(hour FROM ($1::timestamptz AT TIME ZONE u.timezone))*60+
 extract(minute FROM ($1::timestamptz AT TIME ZONE u.timezone)) AS minute) local_time
WHERE o.approved_plan_id IS NULL AND o.status='scheduled' AND o.target_send_at<=$1 AND l.archived_at IS NULL AND u.deleted_at IS NULL
AND (NOT u.quiet_enabled OR NOT CASE WHEN u.quiet_start<u.quiet_end
 THEN local_time.minute>=u.quiet_start AND local_time.minute<u.quiet_end
 ELSE local_time.minute>=u.quiet_start OR local_time.minute<u.quiet_end END)
ORDER BY o.target_send_at,o.id LIMIT $2;

-- name: MarkPreferenceReminderDelivered
UPDATE reminder_occurrences SET status='satisfied'
WHERE id=$1 AND status='scheduled' AND target_send_at<=$2;
