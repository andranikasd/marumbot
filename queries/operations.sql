-- name: QueueStatus
-- Routine probes must not count growing financial history tables.
SELECT
  (SELECT count(*) FROM telegram_commands WHERE status = 'pending'),
  ((SELECT count(*) FROM reminder_occurrences WHERE status = 'scheduled') + (SELECT count(*) FROM notification_deliveries WHERE delivery_kind='loan_filed' AND status IN ('pending','leased'))),
  (SELECT coalesce(greatest(0, extract(epoch FROM now() - min(next_attempt_at))), 0)::bigint
     FROM telegram_commands WHERE status = 'pending' AND next_attempt_at <= now()),
  greatest((SELECT coalesce(greatest(0, extract(epoch FROM now() - min(target_send_at))), 0)::bigint
     FROM reminder_occurrences WHERE status = 'scheduled' AND target_send_at <= now()),
 (SELECT coalesce(greatest(0,extract(epoch FROM now()-min(next_attempt_at))),0)::bigint FROM notification_deliveries WHERE delivery_kind='loan_filed' AND status IN ('pending','leased') AND next_attempt_at<=now()));
