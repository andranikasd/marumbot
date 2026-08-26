-- +goose Up
-- +goose StatementBegin

-- The durable inbox. A table holding only update_id is unsafe: insert the id,
-- crash before applying, and Telegram's retry is discarded as a duplicate. The
-- normalised command and its processing state are what make the retry safe.
--
-- user_id is SET NULL, not CASCADE: erasing a user must not delete the row
-- carrying the permanently unique update id. The payload is cleared instead.
CREATE TABLE telegram_commands (
    id                     uuid PRIMARY KEY,
    telegram_update_id     bigint NOT NULL UNIQUE,
    user_id                uuid REFERENCES users(id) ON DELETE SET NULL,
    command_kind           text NOT NULL,
    command_payload        jsonb NOT NULL DEFAULT '{}',
    payload_schema_version integer NOT NULL,
    trace_context          text,                       -- W3C traceparent
    status                 text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','leased','completed','dead')),
    attempts               smallint NOT NULL DEFAULT 0,
    next_attempt_at        timestamptz NOT NULL DEFAULT now(),
    lease_owner            text,
    lease_token            uuid,
    lease_until            timestamptz,
    received_at            timestamptz NOT NULL DEFAULT now(),
    completed_at           timestamptz,
    last_error_code        text
);
CREATE INDEX telegram_commands_claimable
    ON telegram_commands (next_attempt_at) WHERE status = 'pending';
CREATE INDEX telegram_commands_expired
    ON telegram_commands (lease_until) WHERE status = 'leased';

-- Optimistic-versioned so a redelivered update cannot advance the flow twice.
CREATE TABLE conversation_states (
    user_id                  uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    state_name               text NOT NULL,
    state_version            bigint NOT NULL DEFAULT 0,
    collected                jsonb NOT NULL DEFAULT '{}',
    collected_schema_version integer NOT NULL,
    updated_at               timestamptz NOT NULL DEFAULT now()
);

-- Budgets are per currency. A dram budget cannot fund a dollar loan without an
-- exchange rate, and Marum has no validated source for one, so it plans each
-- currency separately rather than inventing a conversion.
CREATE TABLE budgets (
    user_id                  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    currency                 char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    monthly_amount_minor     bigint NOT NULL CHECK (monthly_amount_minor >= 0),
    reserve_floor_minor      bigint NOT NULL DEFAULT 0 CHECK (reserve_floor_minor >= 0),
    overrides                jsonb NOT NULL DEFAULT '{}',   -- {"2026-12": 40000000}
    overrides_schema_version integer NOT NULL,
    updated_at               timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, currency)
);

CREATE TABLE reminder_rules (
    id            uuid PRIMARY KEY,
    loan_id       uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    offset_days   smallint NOT NULL CHECK (offset_days BETWEEN -30 AND 30),
    send_at_local time NOT NULL DEFAULT '10:00',
    enabled       boolean NOT NULL DEFAULT true,
    UNIQUE (loan_id, offset_days)
);

-- One occurrence is one loan fact due to be reminded. Generated ahead of time
-- and regenerated transactionally when a payment or snapshot changes the loan.
CREATE TABLE reminder_occurrences (
    id              uuid PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    loan_id         uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    due_date        date NOT NULL,
    offset_days     smallint NOT NULL,
    target_send_at  timestamptz NOT NULL,
    status          text NOT NULL DEFAULT 'scheduled'
        CHECK (status IN ('scheduled','attached','satisfied','canceled')),
    idempotency_key text NOT NULL UNIQUE,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reminder_occurrences_due
    ON reminder_occurrences (target_send_at) WHERE status = 'scheduled';

-- One Telegram message may bundle several occurrences for the same user.
CREATE TABLE notification_deliveries (
    id                     uuid PRIMARY KEY,
    user_id                uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delivery_kind          text NOT NULL,
    priority               smallint NOT NULL DEFAULT 100,
    scheduled_at           timestamptz NOT NULL,
    group_key              text NOT NULL UNIQUE,
    payload                jsonb NOT NULL,
    payload_schema_version integer NOT NULL,
    trace_context          text,
    status                 text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','leased','sent','dead','canceled')),
    attempts               smallint NOT NULL DEFAULT 0,
    next_attempt_at        timestamptz NOT NULL,
    lease_owner            text,
    lease_token            uuid,
    lease_until            timestamptz,
    telegram_message_id    bigint,
    sent_at                timestamptz,
    last_error_code        text
);
CREATE INDEX notification_deliveries_claimable
    ON notification_deliveries (priority, next_attempt_at) WHERE status = 'pending';
CREATE INDEX notification_deliveries_expired
    ON notification_deliveries (lease_until) WHERE status = 'leased';

-- An item row IS the attachment, and only a live delivery has one.
--
-- The obvious formulation - a partial unique index predicated on the parent's
-- status - is not expressible: Postgres index predicates may reference only
-- columns of the indexed table, and may not contain a subquery. So the rule is
-- enforced by lifecycle instead: moving a delivery to dead or canceled deletes
-- its item rows in the same transaction, which frees every still-valid
-- occurrence to be regrouped on a later tick. The delivery row keeps the
-- history; the item row only records a current attachment.
CREATE TABLE notification_delivery_items (
    delivery_id   uuid NOT NULL REFERENCES notification_deliveries(id) ON DELETE CASCADE,
    occurrence_id uuid NOT NULL REFERENCES reminder_occurrences(id) ON DELETE CASCADE,
    attached_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (delivery_id, occurrence_id),
    UNIQUE (occurrence_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Billing is a ledger too; entitlement is derived and never hand-edited.
CREATE TABLE billing_events (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider     text NOT NULL,
    kind         text NOT NULL CHECK (kind IN
        ('pre_checkout','purchase','renewal','cancellation','refund','chargeback','manual_grant')),
    external_id  text NOT NULL UNIQUE,
    amount_minor bigint,
    currency     char(3) CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$'),
    occurred_at  timestamptz NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX billing_events_by_user ON billing_events (user_id, occurred_at DESC);

CREATE TABLE entitlements (
    user_id               uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    tier                  text NOT NULL,
    valid_until           timestamptz,
    derived_from_event_id uuid REFERENCES billing_events(id)
);

-- Survives a restore, so a restored dump cannot resurrect an erased account.
CREATE TABLE deletion_tombstones (
    subject_hmac text PRIMARY KEY,
    deleted_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id         bigserial PRIMARY KEY,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    action     text NOT NULL,
    metadata   jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_by_user ON audit_log (user_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS deletion_tombstones;
DROP TABLE IF EXISTS entitlements;
DROP TABLE IF EXISTS billing_events;
DROP TABLE IF EXISTS notification_delivery_items;
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS reminder_occurrences;
DROP TABLE IF EXISTS reminder_rules;
DROP TABLE IF EXISTS budgets;
DROP TABLE IF EXISTS conversation_states;
DROP TABLE IF EXISTS telegram_commands;
-- +goose StatementEnd
