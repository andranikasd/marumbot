# Marum — agent brief

Telegram loan repayment planner. Go. Deterministic arithmetic, no AI in the
product. Full conventions: `docs/engineering-guide.md`. Design:
`docs/design/Marum-MVP-Design.pdf`.

## Layout

```
cmd/marum/        wiring only, no logic
pkg/core/         pure engine: money, model, amort, alloc, ledger, rates, plan
internal/app/     use cases; owns the port interfaces
internal/adapter/ in/{telegram,httpapi}  out/{postgres,telegramclient,blob,sysclock}
migrations/       goose, expand-only        queries/  sqlc input
testdata/golden/  real lender schedules — the correctness corpus
```

Dependencies point inward: adapters → app → `pkg/core`. `pkg/core` imports
nothing from `internal/`.

## Five invariants — do not violate, do not change alone

1. **Money is `int64` minor units.** Never `float64`. Only `pkg/core/rates` may
   use floats at all, and it returns rates, not amounts.
2. **`time.Now()` exists only in `internal/adapter/out/sysclock`.** Time enters
   the engine as a parameter.
3. **`pkg/core` does no I/O** — no database, no network, no clock, no
   randomness. It must stay compilable on its own.
4. **`loan_events` and `billing_events` are append-only.** A mistake is a
   `reversal` row, never an `UPDATE` or `DELETE`. `loan_state` is a rebuildable
   cache guarded by an optimistic-lock `version` column.
5. **No amount or identifier in a log or a metric label.** Balances, chat IDs
   and user UUIDs stay out of telemetry; use a request correlation ID.

`.golangci.yml` enforces all five. If lint blocks you, the rule is right and the
code is wrong.

## Commands

```bash
make run        # local, long-polling against a dev bot token
make test       # unit + golden + integration, always -race
make lint       # gofumpt + golangci-lint
make migrate    # goose up against the local database
```

## House style

- SQL only in `queries/*.sql`, compiled by sqlc. No SQL strings in Go.
- Interfaces declared by the consumer in `internal/app/ports.go`, kept small.
- Transactions opened in `internal/app`, never in an adapter. No network call
  inside a transaction.
- No `go func()` without an owner that can wait for it and stop it.
- No `time.Sleep` in tests. Inject the fake clock.
- No mock frameworks; hand-written fakes in `internal/app/apptest`.
- No `utils`, `helpers`, `common`, or `shared` packages.
- Conventional Commits, subject ≤ 50 chars, `git commit -s` for DCO.

## Needs a human

- Changing any of the five invariants.
- Any number shown to a user without a golden fixture behind it.
- Payment allocation behaviour for a lender we have not studied — the default
  is `unknown/v0`, which asks the user for the bank-reported balance rather
  than deriving one.
- Anything that persists data which could be recomputed.
