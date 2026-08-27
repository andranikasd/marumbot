-- name: EnqueueCommand
-- Idempotent on the Telegram update id. Delivery is at-least-once, so the same
-- update will arrive twice; the second arrival must change nothing. ON CONFLICT
-- DO NOTHING makes that a property of the schema rather than of the caller
-- remembering to check first.
INSERT INTO telegram_commands (
    id, telegram_update_id, user_id, command_kind, command_payload,
    payload_schema_version, trace_context, status, next_attempt_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', now())
ON CONFLICT (telegram_update_id) DO NOTHING
RETURNING id;

-- name: LeaseCommands
-- Claim due work.
--
-- FOR UPDATE SKIP LOCKED is what lets several workers drain the same queue
-- without coordinating: each takes rows nobody else holds, and none of them
-- blocks waiting for a row another is already handling.
--
-- The lease token is fresh on every claim. A worker whose lease expires mid-work
-- will find its token no longer matches and fail its write, instead of
-- overwriting whatever the worker that replaced it has since done.
WITH due AS (
    SELECT id
      FROM telegram_commands
     WHERE status IN ('pending', 'leased')
       AND next_attempt_at <= now()
       AND (lease_until IS NULL OR lease_until < now())
     ORDER BY next_attempt_at
     LIMIT $2
     FOR UPDATE SKIP LOCKED
)
UPDATE telegram_commands c
   SET status      = 'leased',
       lease_owner = $1,
       lease_token = gen_random_uuid(),
       lease_until = $3,
       attempts    = c.attempts + 1
  FROM due
 WHERE c.id = due.id
RETURNING c.id, c.telegram_update_id, coalesce(c.user_id::text, ''), c.command_kind,
          c.command_payload, coalesce(c.trace_context, ''), c.attempts,
          c.received_at, c.lease_token, c.lease_until;

-- name: CompleteCommand
-- The lease_token predicate is the fence. Without it a stalled worker could
-- complete a command that another worker has already retried, and the retry's
-- effects would be silently accepted as the older worker's.
UPDATE telegram_commands
   SET status = 'completed', completed_at = now(),
       lease_owner = NULL, lease_token = NULL, lease_until = NULL
 WHERE id = $1 AND lease_token = $2
RETURNING id;

-- name: FailCommand
UPDATE telegram_commands
   SET status          = CASE WHEN $4 THEN 'dead' ELSE 'pending' END,
       next_attempt_at = $5,
       last_error_code = $3,
       lease_owner     = NULL, lease_token = NULL, lease_until = NULL
 WHERE id = $1 AND lease_token = $2
RETURNING id;

-- name: UpsertUserByTelegram
-- Resolve a Telegram user to an account, creating one on first contact.
--
-- The Telegram identifiers never reach any financial table. identities holds
-- them encrypted, keyed separately, so a leak of loan data does not also reveal
-- whose loans they are. The lookup goes through an HMAC of the id: deterministic
-- enough to find the row, not reversible into the id itself.
--
-- $1 user hmac, $2 new user uuid, $3 locale, $4 timezone, $5 trial ends at,
-- $6 user ciphertext, $7 chat hmac, $8 chat ciphertext, $9 key version.
WITH found AS (
    SELECT user_id FROM identities WHERE telegram_user_hmac = $1
), created AS (
    INSERT INTO users (id, locale, timezone, trial_ends_at)
    SELECT $2, $3, $4, $5
     WHERE NOT EXISTS (SELECT 1 FROM found)
    RETURNING id
), linked AS (
    INSERT INTO identities (
        user_id, telegram_user_enc, telegram_user_hmac,
        telegram_chat_enc, telegram_chat_hmac, key_version)
    SELECT id, $6, $1, $8, $7, $9 FROM created
    RETURNING user_id
)
SELECT user_id, false AS created FROM found
UNION ALL
SELECT user_id, true  AS created FROM linked;

-- name: GetUserLocale
SELECT locale, timezone FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: SetUserLocale
UPDATE users SET locale = $2 WHERE id = $1 AND deleted_at IS NULL RETURNING id;

-- name: GetChatCipher
-- The chat id comes back encrypted; decryption happens above the adapter, so
-- this package never holds a Telegram identifier in the clear.
SELECT telegram_chat_enc, key_version FROM identities WHERE user_id = $1;

-- name: GetUserByTelegramTag
-- Finds an existing account only. The Mini App is reachable only from a bot
-- message, so an account that does not exist means something is wrong rather
-- than something new.
SELECT user_id FROM identities WHERE telegram_user_hmac = $1;

-- name: LeaseCommandByID
-- Claim one specific command, by the id the webhook just wrote.
--
-- The webhook must answer the message in front of it, not whichever command
-- happens to be oldest. Leasing oldest-first there means one command that keeps
-- failing holds the slot and every new message queues behind it -- which is a
-- backlog that looks exactly like the bot being slow.
UPDATE telegram_commands c
   SET status      = 'leased',
       lease_owner = $2,
       lease_token = gen_random_uuid(),
       lease_until = $3,
       attempts    = c.attempts + 1
 WHERE c.id = $1
   AND c.status IN ('pending', 'leased')
   AND (c.lease_until IS NULL OR c.lease_until < now())
RETURNING c.id, c.telegram_update_id, coalesce(c.user_id::text, ''), c.command_kind,
          c.command_payload, coalesce(c.trace_context, ''), c.attempts,
          c.received_at, c.lease_token, c.lease_until;

-- name: SetConversationState
-- What the bot is waiting for from this user. One row per user: a bot that is
-- waiting for two things at once cannot know which answer it just received.
INSERT INTO conversation_states (user_id, state_name, collected, collected_schema_version)
VALUES ($1, $2, '{}'::jsonb, 1)
ON CONFLICT (user_id) DO UPDATE
   SET state_name = EXCLUDED.state_name,
       state_version = conversation_states.state_version + 1,
       updated_at = now()
RETURNING state_name;

-- name: GetConversationState
-- Stale states are ignored rather than deleted. A user who was asked for a
-- budget an hour ago and now types an unrelated number should not have it
-- silently taken as an answer.
SELECT state_name FROM conversation_states
 WHERE user_id = $1 AND updated_at > now() - interval '30 minutes';

-- name: ClearConversationState
DELETE FROM conversation_states WHERE user_id = $1 RETURNING user_id;
