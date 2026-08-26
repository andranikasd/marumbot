-- +goose Up
-- +goose StatementBegin

-- Money is bigint minor units with the currency on the owning row; rates are
-- numeric(12,9); business dates are `date` and instants are `timestamptz`.
-- Enumerations are text with a CHECK rather than Postgres enums, because
-- adding a value to an enum is a migration hazard for no benefit here.

CREATE TABLE users (
    id                    uuid PRIMARY KEY,
    locale                text NOT NULL DEFAULT 'hy' CHECK (locale IN ('hy','en','ru')),
    timezone              text NOT NULL DEFAULT 'Asia/Yerevan',
    trial_ends_at         timestamptz NOT NULL,
    access_state          text NOT NULL DEFAULT 'trial'
        CHECK (access_state IN ('trial','grace','active','paused')),
    created_at            timestamptz NOT NULL DEFAULT now(),
    deletion_requested_at timestamptz,
    deleted_at            timestamptz
);

-- Telegram identifiers live apart from financial records and are encrypted.
-- key_version exists so the key can be rotated without a guessing game across
-- a table nobody can read.
CREATE TABLE identities (
    user_id            uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    telegram_user_enc  bytea NOT NULL,
    telegram_user_hmac text  NOT NULL UNIQUE,
    telegram_chat_enc  bytea NOT NULL,
    telegram_chat_hmac text  NOT NULL UNIQUE,
    key_version        smallint NOT NULL,
    linked_at          timestamptz NOT NULL DEFAULT now()
);

-- Currency is validated against the engine's registry, not by an enum here:
-- exponent and settlement unit are code, and a CHECK listing codes would drift.
CREATE TABLE loans (
    id             uuid PRIMARY KEY,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name           text NOT NULL,
    lender         text,
    currency       char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    next_event_seq bigint NOT NULL DEFAULT 1,
    created_at     timestamptz NOT NULL DEFAULT now(),
    archived_at    timestamptz
);
CREATE INDEX loans_by_user ON loans (user_id) WHERE archived_at IS NULL;

-- Where a payment goes is a bank fact, versioned, sourced from a real contract.
CREATE TABLE allocation_policy_versions (
    id                        uuid PRIMARY KEY,
    policy_key                text NOT NULL,
    version                   integer NOT NULL,
    definition                jsonb NOT NULL,
    definition_schema_version integer NOT NULL,
    excess_rule               text NOT NULL
        CHECK (excess_rule IN ('reduce_principal','hold_as_advance','requires_bank_request','unknown')),
    source_reference          text NOT NULL,
    created_at                timestamptz NOT NULL DEFAULT now(),
    UNIQUE (policy_key, version)
);

CREATE TABLE loan_contract_versions (
    id                           uuid PRIMARY KEY,
    loan_id                      uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    version                      integer NOT NULL,
    effective_from               date NOT NULL,
    effective_until              date,
    nominal_rate                 numeric(12,9) NOT NULL CHECK (nominal_rate >= 0),
    day_count                    text NOT NULL CHECK (day_count IN ('act365','act360','30_360')),
    repayment_type               text NOT NULL CHECK (repayment_type IN ('annuity')),
    start_date                   date NOT NULL,
    maturity_date                date NOT NULL,
    payment_day                  smallint NOT NULL CHECK (payment_day BETWEEN 1 AND 31),
    scheduled_payment_minor      bigint CHECK (scheduled_payment_minor IS NULL OR scheduled_payment_minor > 0),
    rounding_mode                text NOT NULL CHECK (rounding_mode IN ('half_up','half_even','down','up')),
    rounding_unit_minor          integer NOT NULL CHECK (rounding_unit_minor >= 1),
    allocation_policy_version_id uuid NOT NULL REFERENCES allocation_policy_versions(id),
    prepayment_policy            jsonb NOT NULL DEFAULT '{}',
    prepayment_schema_version    integer NOT NULL,
    created_at                   timestamptz NOT NULL DEFAULT now(),
    CHECK (effective_until IS NULL OR effective_until >= effective_from),
    CHECK (maturity_date > start_date),
    UNIQUE (loan_id, version)
);
CREATE INDEX loan_contracts_effective ON loan_contract_versions (loan_id, effective_from DESC);

-- What the lender said, on a stated date. Never inferred.
CREATE TABLE loan_snapshots (
    id                      uuid PRIMARY KEY,
    loan_id                 uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    contract_version_id     uuid NOT NULL REFERENCES loan_contract_versions(id),
    as_of                   date NOT NULL,
    captured_at             timestamptz NOT NULL DEFAULT now(),
    trust                   text NOT NULL
        CHECK (trust IN ('user_entered','bank_confirmed','imported_verified')),
    principal_minor         bigint NOT NULL CHECK (principal_minor >= 0),
    accrued_interest_minor  bigint NOT NULL DEFAULT 0,
    unpaid_interest_minor   bigint NOT NULL DEFAULT 0,
    current_fees_minor      bigint NOT NULL DEFAULT 0,
    overdue_fees_minor      bigint NOT NULL DEFAULT 0,
    penalties_minor         bigint NOT NULL DEFAULT 0,
    overdue_principal_minor bigint NOT NULL DEFAULT 0,
    advance_credit_minor    bigint NOT NULL DEFAULT 0,
    next_installment_minor  bigint,
    next_due_date           date,
    remaining_installments  smallint,
    source_note             text,
    idempotency_key         text NOT NULL UNIQUE
);
CREATE INDEX loan_snapshots_latest ON loan_snapshots (loan_id, as_of DESC, captured_at DESC);

-- The ledger. Append-only: no UPDATE, no DELETE. A mistake is a void row.
CREATE TABLE loan_events (
    id                  uuid PRIMARY KEY,
    loan_id             uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    contract_version_id uuid NOT NULL REFERENCES loan_contract_versions(id),
    recorded_seq        bigint NOT NULL,
    kind                text NOT NULL CHECK (kind IN (
        'payment_reported','prepayment_reported','bank_fee_reported',
        'entry_voided','loan_closed_reported')),
    value_date          date NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),
    amount_minor        bigint,
    bank_order          integer,
    bank_reference      text,
    voids_event_id      uuid REFERENCES loan_events(id),
    source_command_id   uuid,
    idempotency_key     text NOT NULL UNIQUE,
    fact_payload        jsonb NOT NULL DEFAULT '{}',
    fact_schema_version integer NOT NULL,
    UNIQUE (loan_id, recorded_seq)
);
CREATE INDEX loan_events_replay
    ON loan_events (loan_id, value_date, bank_order NULLS LAST, recorded_seq);

-- The user's assertion that a snapshot already includes an event. Separate so
-- the immutable event row never needs an UPDATE.
CREATE TABLE snapshot_event_coverage (
    snapshot_id       uuid NOT NULL REFERENCES loan_snapshots(id) ON DELETE CASCADE,
    event_id          uuid NOT NULL REFERENCES loan_events(id) ON DELETE CASCADE,
    confirmed_at      timestamptz NOT NULL DEFAULT now(),
    source_command_id uuid,
    PRIMARY KEY (snapshot_id, event_id),
    UNIQUE (event_id)
);

-- How a payment was interpreted. Superseded, never rewritten.
CREATE TABLE loan_event_allocations (
    id                           uuid PRIMARY KEY,
    event_id                     uuid NOT NULL REFERENCES loan_events(id) ON DELETE CASCADE,
    replay_generation            uuid NOT NULL,
    contract_version_id          uuid NOT NULL REFERENCES loan_contract_versions(id),
    allocation_policy_version_id uuid NOT NULL REFERENCES allocation_policy_versions(id),
    engine_version               text NOT NULL,
    confidence                   text NOT NULL CHECK (confidence IN ('estimated','bank_confirmed')),
    interest_minor               bigint NOT NULL DEFAULT 0,
    fees_minor                   bigint NOT NULL DEFAULT 0,
    penalties_minor              bigint NOT NULL DEFAULT 0,
    overdue_principal_minor      bigint NOT NULL DEFAULT 0,
    scheduled_principal_minor    bigint NOT NULL DEFAULT 0,
    extra_principal_minor        bigint NOT NULL DEFAULT 0,
    advance_credit_minor         bigint NOT NULL DEFAULT 0,
    supersedes_allocation_id     uuid REFERENCES loan_event_allocations(id),
    calculated_at                timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX loan_event_allocations_history
    ON loan_event_allocations (event_id, calculated_at DESC);

-- Rebuildable cache. state_version is the optimistic lock.
CREATE TABLE loan_state (
    loan_id                 uuid PRIMARY KEY REFERENCES loans(id) ON DELETE CASCADE,
    state_version           bigint NOT NULL DEFAULT 0,
    anchor_snapshot_id      uuid NOT NULL REFERENCES loan_snapshots(id),
    replay_generation       uuid NOT NULL,
    event_set_hash          bytea NOT NULL,
    last_recorded_seq       bigint NOT NULL,
    balance_as_of           date NOT NULL,
    principal_minor         bigint NOT NULL,
    accrued_interest_minor  bigint NOT NULL DEFAULT 0,
    unpaid_interest_minor   bigint NOT NULL DEFAULT 0,
    current_fees_minor      bigint NOT NULL DEFAULT 0,
    overdue_fees_minor      bigint NOT NULL DEFAULT 0,
    penalties_minor         bigint NOT NULL DEFAULT 0,
    overdue_principal_minor bigint NOT NULL DEFAULT 0,
    advance_credit_minor    bigint NOT NULL DEFAULT 0,
    reliability_state       text NOT NULL CHECK (reliability_state IN
        ('confirmed','estimated','stale','needs_reconciliation','unsupported')),
    reliability_reasons     jsonb NOT NULL DEFAULT '[]',
    engine_version          text NOT NULL,
    rebuilt_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE reconciliation_runs (
    id                     uuid PRIMARY KEY,
    loan_id                uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    previous_state_version bigint,
    new_snapshot_id        uuid NOT NULL REFERENCES loan_snapshots(id),
    principal_drift_minor  bigint NOT NULL,
    interest_drift_minor   bigint NOT NULL,
    fee_drift_minor        bigint NOT NULL,
    penalty_drift_minor    bigint NOT NULL,
    engine_version         text NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reconciliation_runs_by_loan ON reconciliation_runs (loan_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS reconciliation_runs;
DROP TABLE IF EXISTS loan_state;
DROP TABLE IF EXISTS loan_event_allocations;
DROP TABLE IF EXISTS snapshot_event_coverage;
DROP TABLE IF EXISTS loan_events;
DROP TABLE IF EXISTS loan_snapshots;
DROP TABLE IF EXISTS loan_contract_versions;
DROP TABLE IF EXISTS allocation_policy_versions;
DROP TABLE IF EXISTS loans;
DROP TABLE IF EXISTS identities;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
