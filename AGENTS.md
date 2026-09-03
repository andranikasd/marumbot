# Marum — agent brief

Telegram loan repayment planner. Go. Deterministic arithmetic, no AI in the
product. Current state: `docs/current-state.md`. Full conventions:
`docs/engineering-guide.md`. Current UI: `docs/design/ui-v1.1.md`.

## Layout

```
cmd/marum/        wiring only, no logic
pkg/core/         pure engine: money, date, model, amortisation, allocation, ledger, plan
internal/app/     use cases; owns the port interfaces
internal/adapter/ in/{telegram,httpapi,miniapp,admin}  out/{postgres,telegramclient,sysclock}
internal/design/  the one design-token sheet both UIs prepend to their stylesheet
migrations/       goose, expand-only        queries/  embedded named SQL
testdata/golden/  lender/reference schedules — scoped correctness corpus
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

Lint, database guards, tests and review enforce these together; lint alone is
not complete proof. The historical `pkg/core/rates` reference has no matching
package today; see the engineering guide for the enforcement gap. Do not
change the invariants to resolve that discrepancy without human review.

## Commands

```bash
make up-core    # local app and database; long polling
make test       # Go suite with -race; DB tests need TEST_DATABASE_URL
make test-store # real PostgreSQL integration; CI also runs it with -race
make lint       # golangci-lint; make fmt applies gofumpt
make migrate    # goose up against the local database
```

## House style

- SQL only in `queries/*.sql`, embedded and loaded by name via `queries/embed.go`. No SQL strings in Go.
- Interfaces declared by the consumer next to their use cases in `internal/app`, kept small.
- Transactions opened in `internal/app`, never in an adapter. No network call
  inside a transaction.
- No `go func()` without an owner that can wait for it and stop it.
- No `time.Sleep` in tests. Inject the fake clock.
- No mock frameworks; hand-written fakes alongside the tests.
- No `utils`, `helpers`, `common`, or `shared` packages.
- Conventional Commits, subject ≤ 50 chars, `git commit -s` for DCO.

## Needs a human

- Changing any of the five invariants.
- Any number shown to a user without a golden fixture behind it.
- Payment allocation behaviour for a lender we have not studied — the default
  is `unknown/v0`, which asks the user for the bank-reported balance rather
  than deriving one.
- Anything that persists data which could be recomputed.
