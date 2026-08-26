# Marum · Մարում

**Pay off smarter.** An open-source Telegram bot that keeps your loans in one
place, reminds you before every payment, and works out how to clear them for
the least money.

No AI. Every figure comes from a published formula over data you entered, and
the same inputs always produce the same answer.

> Marum is a loan ledger and planner. It does not move money, touch bank
> accounts, refinance, recommend lenders, or replace the lender's official
> balance and repayment schedule.

---

## Status

**Phase 1 — building the calculation engine.** Nothing is deployed and there is
no public bot yet.

| Phase | Scope | State |
| --- | --- | --- |
| 0 | Collect real Armenian schedules and lender policies | Not started |
| **1** | **Money, dates, dated accrual, ledger replay** | **In progress** |
| 2 | Durable Telegram inbox, one loan, reminders, payment recording | Not started |
| 3 | Multiple loans, repayment strategies | Not started |
| 4 | Test environment, Telegram Stars, entitlements | Not started |
| 5 | Fee-aware optimiser, further repayment types | Not started |

Phase 1 cannot end until **ten real repayment schedules from four lenders
reproduce to the dram**. That gate sits in front of every user-facing number.

---

## Quick start

Requires Docker with the Compose plugin. No local Go toolchain is needed —
everything builds and tests inside a container.

```bash
cp .env.example .env        # add a Telegram bot token from @BotFather
make up                     # postgres + marum, in polling mode
make test                   # unit + golden tests, race detector on
make logs                   # follow the application log
```

The bot runs in **long-polling** mode locally, so no public URL or tunnel is
required. Production uses webhooks; everything below the transport is the same
code.

| Service | Address | Purpose |
| --- | --- | --- |
| `marum` | `127.0.0.1:8080` | health, readiness, status endpoints |
| `admin` | `127.0.0.1:8081` | private web admin interface |
| `postgres` | `127.0.0.1:5432` | local database |

---

## How it works

Marum keeps four things apart, because they change for different reasons:

1. **Contract terms** — immutable, versioned. A restructuring is a new version.
2. **Bank snapshots** — what the lender reported, on a stated date. Never
   inferred, and only a *confirmed* snapshot resets drift.
3. **Loan events** — append-only. What the user reported happened. A mistake is
   voided by another row, never edited.
4. **Derived state** — a rebuildable cache. Delete it and replay produces it
   again, byte for byte.

Repayment schedules and plans are **projections**: computed on demand, never
stored. If Marum cannot reconstruct a balance it trusts, it says so and asks for
the bank's figure instead of guessing.

Two invariants matter more than the rest:

- **Money is `int64` minor units.** Never a float. Interest accrual runs through
  a 128-bit intermediate because `principal × rate × days` overflows 64 bits
  above roughly 16.5M AMD at 18% — ordinary mortgage territory here.
- **Telegram delivery is at-least-once.** The gap between Telegram accepting a
  message and Marum recording it cannot be closed, so reminders are worded to
  read correctly if one arrives twice.

Currencies are supported from the start. A currency's ISO exponent and its
settlement unit are separate facts: AMD has two decimal places on paper but
settles in whole drams, the yen has none at all, and the dinar has three.
Assuming two everywhere is a factor-of-100 error in one direction and a lost
digit in the other, so unknown codes are rejected rather than guessed.

Budgets and plans are **per currency**. Allocating a dram budget across a
dollar loan needs an exchange rate, Marum has no validated source for one, and
a plan that silently moves with the market is exactly the confident wrong
answer the design refuses.

---

## Repository layout

```
cmd/marum/            wiring only — flags, config, signals, shutdown
pkg/core/             the engine: pure, deterministic, no I/O
  money/              Amount, Rate, rounding policy, dated accrual
  date/               date-only arithmetic and month-end clamping
  model/              Contract, Snapshot, Event, LoanState
  ledger/             replay(contract, snapshot, events) -> state
  amortisation/       dated schedules and the payment solver
  allocation/         per-lender payment allocation policies
  planning/           repayment strategies
internal/
  app/                use cases; owns the port interfaces
  adapter/in/         telegram, httpapi, admin
  adapter/out/        postgres, telegramclient, objectstore, sysclock
  obs/                OpenTelemetry, slog redaction, metrics
  config/             environment parsing and validation
migrations/           goose SQL, expand-only
queries/              sqlc input — no SQL strings in Go
testdata/golden/      real lender schedules and expected output
deploy/               Dockerfile, compose, self-hosting notes
docs/                 design documents and diagrams
```

Dependencies point inward: adapters depend on `internal/app`, which depends on
`pkg/core`. The engine imports nothing from `internal/`, which is enforced by
`depguard` in [`.golangci.yml`](.golangci.yml) and by a test binary that links
only `pkg/core`.

---

## Documentation

| Document | What it covers |
| --- | --- |
| [MVP System and Architecture Design](docs/design/Marum-MVP-System-and-Architecture-Design.pdf) | The current design. Architecture, domain model, delivery semantics, schema, observability. Start here. |
| [Reliable MVP Design v0.3.1](docs/Marum-Reliable-MVP-Design.md) | The long-form reference — full DDL, reliability invariants, failure analysis, revision history. |
| [Engineering guide](docs/engineering-guide.md) | How the code is written: structure, style, testing, the five invariants and what enforces each. |
| [AGENTS.md](AGENTS.md) | The one-page version for AI coding agents. |
| [Diagrams](docs/diagrams/) | Editable draw.io sources for every figure. |

Rebuild the design PDF after changing a diagram:

```bash
docs/diagrams/build.sh && docs/design/build.sh
```

---

## Development

```bash
make up          # start postgres and the app
make down        # stop everything, keep the volume
make reset       # stop and destroy the database volume
make test        # go test ./... -race
make lint        # gofumpt + golangci-lint
make migrate     # apply pending migrations
make migrate-check  # prove the newest migration is reversible: up, down, up
make shell       # psql into the local database
```

All targets run inside containers, so the only requirement is Docker.

### Testing

| Layer | Proves |
| --- | --- |
| Golden schedules | Marum reproduces a real lender's schedule to the dram |
| Property tests | Money never appears or disappears across allocation buckets |
| Ledger replay | `replay(events) == loan_state`, including under concurrency |
| Integration | Real Postgres: leases, idempotency, migration reversibility |
| Journeys | Whole conversations against a fake Telegram transport |

Every user-reported calculation discrepancy becomes a golden fixture **before**
the fix is written. The corpus is the asset.

---

## Observability

Five signals go to Grafana Cloud from the first commit: traces, metrics, logs,
continuous profiles and one synthetic check. A single trace covers a user
action end to end — webhook, command worker, sender — because the W3C
`traceparent` is persisted alongside the work in the durable queues.

Set the OTLP variables in `.env` and it starts reporting. Leave
`OTEL_EXPORTER_OTLP_ENDPOINT` empty and telemetry is disabled entirely, which
is the right default for self-hosters.

The whole design fits inside the Grafana Cloud **free tier** by construction:
roughly 2,300 metric series, 2 GB of traces and 0.3 GB of logs per month at
5,000 users. Section 11 of the design document explains the rules that keep it
there — the binding constraints are configuration choices, not volume.

---

## Privacy

Marum never asks for bank credentials, card numbers, CVV codes, passport or
social-card numbers. Input resembling any of those is rejected without being
stored.

Telegram identifiers are encrypted and kept in a separate table from financial
records. No amount, chat ID or lender name ever reaches a log or a metric
label. Export and account deletion are always available and always free, and a
restored backup cannot resurrect an erased account.

---

## Licence

AGPL-3.0-or-later. The hosted service sells operation — managed infrastructure,
reliable delivery, backups, support — not computation. Anyone running it
themselves gets exactly the same arithmetic.

Contributions are welcome under the Developer Certificate of Origin: sign off
commits with `git commit -s`.
